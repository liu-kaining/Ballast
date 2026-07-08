package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ballast/ballast-server/internal/api"
	"github.com/ballast/ballast-server/internal/automation"
	"github.com/ballast/ballast-server/internal/config"
	"github.com/ballast/ballast-server/internal/notify"
	"github.com/ballast/ballast-server/internal/orchestrator"
	"github.com/ballast/ballast-server/internal/policy/opa"
	dockerruntime "github.com/ballast/ballast-server/internal/runtime/docker"
	"github.com/ballast/ballast-server/internal/store"
	"github.com/ballast/ballast-server/migrations"
)

func main() {
	configPath := flag.String("config", "configs/ballast.yaml", "path to ballast.yaml")
	flag.Parse()

	logger := log.New(os.Stdout, "[ballast] ", log.LstdFlags|log.Lshortfile)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}
	logger.Printf("config loaded from %s (env=%s, runtime=%s)", *configPath, cfg.Server.Environment, cfg.RuntimeProvider.Type)

	// 数据层
	dbStore, err := store.New(context.Background(), cfg.Database.DSN)
	if err != nil {
		logger.Fatalf("connect db: %v (tip: ensure postgres is reachable per docker-compose)", err)
	}
	defer dbStore.Close()
	if err := migrations.Apply(context.Background(), dbStore.Pool()); err != nil {
		logger.Fatalf("apply database migrations: %v", err)
	}
	logger.Printf("db connected")

	// 策略引擎（OPA/Rego）
	regoDir := cfg.Policy.RegoDir
	if regoDir == "" {
		regoDir = "./policies"
	}
	polEng, err := opa.New(regoDir)
	if err != nil {
		logger.Fatalf("load policies: %v", err)
	}
	logger.Printf("policy engine ready (rego_dir=%s)", regoDir)

	// 隔离执行面（Docker CLI + disposable sandbox）
	runtimeCfg := cfg.RuntimeProvider.Config
	sandboxRuntime, err := dockerruntime.New(context.Background(), dockerruntime.Config{
		MaxCPUCores:                runtimeCfg.MaxCPUCores,
		MaxMemoryMB:                runtimeCfg.MaxMemoryMB,
		DefaultImage:               runtimeCfg.DefaultImage,
		WorkspaceRoot:              runtimeCfg.WorkspaceRoot,
		ControlPlaneURL:            runtimeCfg.ControlPlaneURL,
		InternalToken:              cfg.Server.InternalToken,
		RunnerCommand:              runtimeCfg.RunnerCommand,
		RunnerArgs:                 runtimeCfg.RunnerArgs,
		KubeconfigPath:             runtimeCfg.KubeconfigPath,
		RewriteLocalhostKubeconfig: runtimeCfg.RewriteLocalhostKubeconfig,
		KubeNamespace:              runtimeCfg.KubeNamespace,
		KubeTargetSelector:         runtimeCfg.KubeTargetSelector,
		KubeTargetDeployment:       runtimeCfg.KubeTargetDeployment,
		KubeFixConfigMap:           runtimeCfg.KubeFixConfigMap,
	})
	if err != nil {
		logger.Fatalf("initialize sandbox runtime: %v", err)
	}
	logger.Printf("sandbox runtime ready (image=%s)", runtimeCfg.DefaultImage)

	// 编排器
	mgr := orchestrator.New(
		dbStore.Sessions,
		dbStore.Audit,
		sandboxRuntime,
		polEng,
		runtimeCfg.DefaultImage,
		orchestrator.WithSkillRepository(dbStore.Skills),
		orchestrator.WithMCPPluginRepository(dbStore.MCPPlugins),
		orchestrator.WithEventRepository(dbStore.Events),
		orchestrator.WithNotifier(
			notify.NewWebhook(cfg.Notifications.ApprovalWebhookURL, cfg.Notifications.ApprovalWebhookKind),
			cfg.Notifications.ConsoleBaseURL,
		),
		orchestrator.WithWorkspaceRoot(runtimeCfg.WorkspaceRoot),
	)

	// HTTP 路由
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := dbStore.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unhealthy","database":"unavailable"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	api.NewRouter(mux, mgr, logger, api.Options{
		AdminToken:         cfg.Server.AdminToken,
		SessionSecret:      cfg.Server.JWTSecret,
		InternalToken:      cfg.Server.InternalToken,
		CORSAllowedOrigins: cfg.Server.CORSAllowedOrigins,
		CookieSecure:       cfg.Server.CookieSecure,
		Skills:             dbStore.Skills,
		TriggerRules:       dbStore.TriggerRules,
		MCPPlugins:         dbStore.MCPPlugins,
	})

	automationCtx, stopAutomation := context.WithCancel(context.Background())
	defer stopAutomation()
	go automation.NewScheduler(dbStore.TriggerRules, mgr, logger, 30*time.Second).Start(automationCtx)

	srv := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		logger.Printf("ballast-server listening on %s", cfg.Server.Address)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("listen: %v", err)
		}
	}()

	reloadCtx, stopReload := context.WithCancel(context.Background())
	defer stopReload()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-reloadCtx.Done():
				return
			case <-ticker.C:
				if err := polEng.Reload(regoDir); err != nil {
					logger.Printf("policy reload rejected; keeping last valid policy: %v", err)
				}
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Printf("shutdown signal received, draining...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopReload()
	stopAutomation()
	if err := mgr.Shutdown(ctx); err != nil {
		logger.Printf("sandbox shutdown error: %v", err)
	}
	if err := srv.Shutdown(ctx); err != nil {
		logger.Printf("shutdown error: %v", err)
	}
	logger.Printf("bye")
}
