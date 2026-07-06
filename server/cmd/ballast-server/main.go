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
	"github.com/ballast/ballast-server/internal/config"
	"github.com/ballast/ballast-server/internal/opencode/mock"
	"github.com/ballast/ballast-server/internal/orchestrator"
	"github.com/ballast/ballast-server/internal/policy/opa"
	"github.com/ballast/ballast-server/internal/store"
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

	// OpenCode 引擎（v0.1 用 Mock）
	eng := mock.New()

	// 编排器
	mgr := orchestrator.New(dbStore, eng, polEng)

	// HTTP 路由
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	api.NewRouter(mux, mgr, polEng, logger)

	srv := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Printf("ballast-server listening on %s", cfg.Server.Address)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Printf("shutdown signal received, draining...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = eng.Stop(ctx)
	if err := srv.Shutdown(ctx); err != nil {
		logger.Printf("shutdown error: %v", err)
	}
	logger.Printf("bye")
}
