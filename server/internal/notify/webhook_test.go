package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookNotifierPostsGenericPayload(t *testing.T) {
	var got ApprovalNotification
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content-type = %q", req.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	notifier := NewWebhook(server.URL, "generic")
	if err := notifier.NotifyApprovalRequired(context.Background(), ApprovalNotification{
		SessionID: "sess-1",
		Title:     "CrashLoop",
		Command:   "kubectl apply -f fix.yaml",
	}); err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "sess-1" || got.Command == "" {
		t.Fatalf("payload = %#v", got)
	}
}

func TestNoopNotifierWhenURLMissing(t *testing.T) {
	if err := NewWebhook("", "feishu").NotifyApprovalRequired(context.Background(), ApprovalNotification{}); err != nil {
		t.Fatal(err)
	}
}
