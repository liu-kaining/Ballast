// Package notify sends external approval notifications.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ApprovalNotification struct {
	SessionID  string `json:"session_id"`
	Title      string `json:"title"`
	Command    string `json:"command"`
	ConsoleURL string `json:"console_url,omitempty"`
}

type Notifier interface {
	NotifyApprovalRequired(ctx context.Context, notification ApprovalNotification) error
}

type NoopNotifier struct{}

func (NoopNotifier) NotifyApprovalRequired(context.Context, ApprovalNotification) error { return nil }

type WebhookNotifier struct {
	URL        string
	Kind       string
	HTTPClient *http.Client
}

func NewWebhook(url, kind string) Notifier {
	url = strings.TrimSpace(url)
	if url == "" {
		return NoopNotifier{}
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "generic"
	}
	return &WebhookNotifier{
		URL:        url,
		Kind:       kind,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (n *WebhookNotifier) NotifyApprovalRequired(ctx context.Context, notification ApprovalNotification) error {
	body, err := json.Marshal(n.payload(notification))
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("notification webhook returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

func (n *WebhookNotifier) payload(notification ApprovalNotification) any {
	title := "Ballast approval required"
	text := fmt.Sprintf("Session: %s\nCommand: `%s`", notification.SessionID, notification.Command)
	if notification.Title != "" {
		text = fmt.Sprintf("Session: %s (%s)\nCommand: `%s`", notification.SessionID, notification.Title, notification.Command)
	}
	if notification.ConsoleURL != "" {
		text += "\nConsole: " + notification.ConsoleURL
	}
	switch n.Kind {
	case "feishu", "lark":
		return map[string]any{
			"msg_type": "interactive",
			"card": map[string]any{
				"header": map[string]any{
					"title": map[string]string{"tag": "plain_text", "content": title},
				},
				"elements": []any{
					map[string]any{
						"tag":     "div",
						"text":    map[string]string{"tag": "lark_md", "content": text},
						"content": text,
					},
				},
			},
		}
	case "dingtalk":
		return map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"title": title,
				"text":  "### " + title + "\n\n" + text,
			},
		}
	default:
		return notification
	}
}
