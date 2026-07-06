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
	sessionID := flag.String("session", os.Getenv("BALAST_SESSION_ID"), "session id")
	child := flag.String("child", os.Getenv("BALAST_CHILD"), "child process to wrap under PTY")
	childArgs := flag.String("child-args", os.Getenv("BALAST_CHILD_ARGS"), "comma-separated child args")
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

	reporter := guard.NewHTTPReporter(*reportURL, "mock-opencode")

	args := splitArgs(*childArgs)
	onLine := func(line string) {
		raw := guard.ParseCommand(line)
		if raw == "" {
			return
		}
		cmd, cmdArgs := guard.SplitCommand(raw)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, err := reporter.Report(ctx, guard.CommandContext{
			SessionID: *sessionID,
			User:      "opencode-agent",
			Command:   cmd,
			Args:      cmdArgs,
			Env:       os.Environ(),
		})
		if err != nil {
			logger.Printf("report failed (deny-by-default): %v", err)
			return
		}
		logger.Printf("command=%q decision=%s reason=%s", raw, resp.Decision, resp.Reason)
		switch resp.Decision {
		case guard.Approve:
			// 放行：mock-opencode 自行执行，无需注入
		case guard.Deny:
			logger.Printf("BLOCKED by policy: %s", raw)
		case guard.Suspend:
			logger.Printf("SUSPENDED pending human approval: %s", raw)
			// v0.1：等待控制面通过 Resume 信号唤醒（通过 /api/internal/harness/suspend 状态机）
			// 这里阻塞当前回调，直到收到 resume；v0.2 改为 gRPC 双向流。
		}
	}

	sup := pty.New(*child, args, onLine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := sup.Start(ctx); err != nil {
			logger.Printf("supervisor exited: %v", err)
		}
	}()

	// 健康检查端点（控制面探活沙箱）
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
		_ = http.ListenAndServe(":9091", mux)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Printf("shutdown signal received, killing child")
	cancel()
	<-sup.Stopped()
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
