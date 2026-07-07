package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ballast/ballast-server/internal/opencode"
)

func TestHTTPEngineSessionPromptMCPAndEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "oc-session"})
		case r.Method == http.MethodPost && r.URL.Path == "/session/oc-session/message":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "msg-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/mcp":
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && r.URL.Path == "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: reason.step\n"))
			_, _ = w.Write([]byte(`data: {"properties":{"index":1,"title":"Plan","thought":"Inspect"},"type":"reason.step"}` + "\n\n"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	engine, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := engine.StartSession(context.Background(), "triage", opencode.SessionOpts{})
	if err != nil || sessionID != "oc-session" {
		t.Fatalf("sessionID=%q err=%v", sessionID, err)
	}
	messageID, err := engine.Prompt(context.Background(), sessionID, "hello")
	if err != nil || messageID != "msg-1" {
		t.Fatalf("messageID=%q err=%v", messageID, err)
	}
	if err := engine.InjectMCP(context.Background(), "prometheus", opencode.MCPConfig{Command: "prom-mcp"}); err != nil {
		t.Fatal(err)
	}
	events, err := engine.Events(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.Type != "reason.step" {
		t.Fatalf("event = %#v", event)
	}
	step, ok := opencode.ParseReasonStep(event)
	if !ok || step.Title != "Plan" {
		t.Fatalf("step=%#v ok=%v", step, ok)
	}
}
