package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ballast/harness-agent/internal/guard"
	"github.com/ballast/harness-agent/internal/pty"
)

func main() {
	controlAddr := flag.String("control-addr", os.Getenv("BALLAST_CONTROL_ADDR"), "ballast control plane address (host:port)")
	reportURL := flag.String("report-url", os.Getenv("BALLAST_REPORT_URL"), "control plane report endpoint, e.g. http://host:8080/api/internal/harness/report")
	eventURL := flag.String("event-url", os.Getenv("BALLAST_EVENT_URL"), "control plane event endpoint")
	internalToken := flag.String("internal-token", os.Getenv("BALLAST_INTERNAL_TOKEN"), "control plane internal bearer token")
	sessionID := flag.String("session", os.Getenv("BALLAST_SESSION_ID"), "session id")
	child := flag.String("child", os.Getenv("BALLAST_CHILD"), "child process to wrap under PTY")
	childArgs := flag.String("child-args", os.Getenv("BALLAST_CHILD_ARGS"), "comma-separated child args")
	flag.Parse()

	logger := log.New(os.Stdout, "[harness-agent] ", log.LstdFlags|log.Lshortfile)
	logger.Printf("starting session=%s control=%s child=%s args=%s", *sessionID, *controlAddr, *child, *childArgs)

	if *child == "" {
		*child = "/usr/local/bin/mock-opencode"
	}
	if *reportURL == "" && *controlAddr != "" {
		*reportURL = fmt.Sprintf("http://%s/api/internal/harness/report", *controlAddr)
	}
	if *reportURL == "" {
		logger.Fatalf("neither --report-url nor --control-addr provided")
	}
	if *eventURL == "" {
		logger.Fatalf("--event-url is required")
	}
	if *internalToken == "" {
		logger.Fatalf("--internal-token is required")
	}
	if *sessionID == "" {
		logger.Fatalf("--session is required")
	}

	reporter := guard.NewHTTPReporter(*reportURL, "mock-opencode", *internalToken)
	eventReporter := guard.NewHTTPEventReporter(*eventURL, *internalToken)

	args := splitArgs(*childArgs)
	var sup *pty.Supervisor
	onLine := func(line string) {
		event, ok := guard.ParseEvent(line)
		if !ok {
			return
		}
		event.SessionID = *sessionID
		eventCtx, eventCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := eventReporter.Report(eventCtx, event); err != nil {
			logger.Printf("event report failed: %v", err)
		}
		eventCancel()

		raw := guard.ParseCommand(line)
		if raw == "" {
			return
		}
		cmd, cmdArgs, unsafe := guard.AnalyzeCommand(raw)
		resp, err := reporter.Report(context.Background(), guard.CommandContext{
			SessionID: *sessionID,
			User:      "opencode-agent",
			Command:   cmd,
			Args:      cmdArgs,
			Env:       guard.RedactEnvironment(os.Environ()),
			Unsafe:    unsafe,
		})
		if err != nil {
			logger.Printf("report failed (deny-by-default): %v", err)
			resp.Decision = guard.Deny
		}
		logger.Printf("command=%q decision=%s reason=%s", raw, resp.Decision, resp.Reason)
		if resp.Decision == guard.Deny {
			logger.Printf("BLOCKED by policy: %s", raw)
		}
		if _, err := sup.Write([]byte(string(resp.Decision) + "\n")); err != nil {
			logger.Printf("write decision to child: %v", err)
		}
	}

	sup = pty.New(*child, args, onLine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 健康检查端点（控制面探活沙箱）
	healthServer := &http.Server{Addr: ":9091", ReadHeaderTimeout: 5 * time.Second}
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
		healthServer.Handler = mux
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Printf("health server: %v", err)
		}
	}()

	supervisorDone := make(chan error, 1)
	go func() { supervisorDone <- sup.Start(ctx) }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		logger.Printf("shutdown signal %s received, killing child", sig)
		cancel()
		<-sup.Stopped()
	case err := <-supervisorDone:
		if err != nil && err != context.Canceled {
			logger.Printf("supervisor exited: %v", err)
		}
		cancel()
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = healthServer.Shutdown(shutdownCtx)
	shutdownCancel()
	logger.Printf("bye")
}

func splitArgs(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
