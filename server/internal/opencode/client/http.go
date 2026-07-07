// Package client implements the HTTP/SSE OpenCode engine adapter.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ballast/ballast-server/internal/opencode"
)

type HTTPEngine struct {
	baseURL    string
	httpClient *http.Client
}

type Option func(*HTTPEngine)

func WithHTTPClient(client *http.Client) Option {
	return func(engine *HTTPEngine) {
		if client != nil {
			engine.httpClient = client
		}
	}
}

func New(baseURL string, options ...Option) (*HTTPEngine, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid opencode base url %q", baseURL)
	}
	engine := &HTTPEngine{
		baseURL: parsed.String(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, option := range options {
		option(engine)
	}
	return engine, nil
}

func (e *HTTPEngine) StartSession(ctx context.Context, title string, _ opencode.SessionOpts) (string, error) {
	var response struct {
		ID string `json:"id"`
	}
	if err := e.doJSON(ctx, http.MethodPost, "/session", map[string]string{"title": title}, &response); err != nil {
		return "", err
	}
	if response.ID == "" {
		return "", errors.New("opencode returned empty session id")
	}
	return response.ID, nil
}

func (e *HTTPEngine) Prompt(ctx context.Context, sessionID string, text string) (string, error) {
	body := map[string]any{
		"parts": []map[string]string{{"type": "text", "text": text}},
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := e.doJSON(ctx, http.MethodPost, "/session/"+url.PathEscape(sessionID)+"/message", body, &response); err != nil {
		return "", err
	}
	if response.ID == "" {
		return "", errors.New("opencode returned empty message id")
	}
	return response.ID, nil
}

func (e *HTTPEngine) Events(ctx context.Context, _ string) (<-chan opencode.Event, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/event", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := e.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("open opencode event stream: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		return nil, fmt.Errorf("opencode event stream status %d: %s", response.StatusCode, readSmall(response.Body))
	}
	out := make(chan opencode.Event, 32)
	go func() {
		defer close(out)
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var eventName string
		var data bytes.Buffer
		flush := func() bool {
			if data.Len() == 0 {
				eventName = ""
				return true
			}
			event, err := decodeSSEEvent(eventName, data.Bytes())
			data.Reset()
			eventName = ""
			if err != nil {
				return true
			}
			select {
			case out <- event:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if !flush() {
					return
				}
				continue
			}
			if strings.HasPrefix(line, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		_ = flush()
	}()
	return out, nil
}

func (e *HTTPEngine) InjectMCP(ctx context.Context, name string, config opencode.MCPConfig) error {
	body := map[string]any{"name": name, "config": config}
	return e.doJSON(ctx, http.MethodPost, "/mcp", body, nil)
}

func (e *HTTPEngine) Stop(context.Context) error {
	e.httpClient.CloseIdleConnections()
	return nil
}

func (e *HTTPEngine) doJSON(ctx context.Context, method, path string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, e.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := e.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("opencode %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("opencode %s %s status %d: %s", method, path, response.StatusCode, readSmall(response.Body))
	}
	if target == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode opencode response: %w", err)
	}
	return nil
}

func decodeSSEEvent(eventName string, data []byte) (opencode.Event, error) {
	var raw struct {
		Type       string          `json:"type"`
		Properties json.RawMessage `json:"properties"`
		Payload    json.RawMessage `json:"payload"`
		Timestamp  time.Time       `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return opencode.Event{}, err
	}
	if raw.Type == "" {
		raw.Type = eventName
	}
	payload := raw.Properties
	if len(payload) == 0 {
		payload = raw.Payload
	}
	if len(payload) == 0 {
		payload = json.RawMessage(data)
	}
	if raw.Timestamp.IsZero() {
		raw.Timestamp = time.Now().UTC()
	}
	return opencode.Event{
		Type:      raw.Type,
		Payload:   payload,
		Timestamp: raw.Timestamp,
	}, nil
}

func readSmall(reader io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(reader, 4096))
	return strings.TrimSpace(string(data))
}

var _ opencode.Engine = (*HTTPEngine)(nil)
