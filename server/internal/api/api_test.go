package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/store"
)

func TestSessionSignature(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	value := signSession(now.Add(time.Hour), "secret")
	if !verifySession(value, "secret", now) {
		t.Fatal("valid session rejected")
	}
	if verifySession(value+"tampered", "secret", now) {
		t.Fatal("tampered session accepted")
	}
	if verifySession(value, "secret", now.Add(2*time.Hour)) {
		t.Fatal("expired session accepted")
	}
}

func TestLoginSetsHttpOnlyCookieAndCORS(t *testing.T) {
	mux := http.NewServeMux()
	NewRouter(mux, nil, log.New(io.Discard, "", 0), Options{
		AdminToken:         "admin",
		SessionSecret:      "secret",
		InternalToken:      "internal",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	})
	body, _ := json.Marshal(map[string]string{"token": "admin"})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatal("missing CORS allow origin")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatalf("cookies = %#v", cookies)
	}
}

func TestProtectedRouteRejectsAnonymousAndDisallowedOrigin(t *testing.T) {
	mux := http.NewServeMux()
	NewRouter(mux, nil, log.New(io.Discard, "", 0), Options{
		AdminToken:         "admin",
		SessionSecret:      "secret",
		InternalToken:      "internal",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	request.Header.Set("Origin", "https://evil.example")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d", response.Code)
	}
}

func TestDecodeJSONRejectsTrailingValue(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"token":"a"} {"token":"b"}`))
	response := httptest.NewRecorder()
	var target map[string]string
	if err := decodeJSON(response, request, &target); err == nil {
		t.Fatal("expected trailing JSON value to be rejected")
	}
}

func TestSkillAndTriggerRuleAPIs(t *testing.T) {
	mux := http.NewServeMux()
	skills := newMemorySkillRepo()
	rules := newMemoryTriggerRuleRepo()
	NewRouter(mux, nil, log.New(io.Discard, "", 0), Options{
		AdminToken:         "admin",
		SessionSecret:      "secret",
		InternalToken:      "internal",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		Skills:             skills,
		TriggerRules:       rules,
	})
	cookie := &http.Cookie{Name: sessionCookieName, Value: signSession(time.Now().Add(time.Hour), "secret")}

	skillBody := `{"skill_id":"k8s-debug","name":"K8s Debug","trigger_words":["pod","pod"],"markdown_content":"---\nname: k8s-debug\n---\n# Debug","updated_by":"tester"}`
	request := httptest.NewRequest(http.MethodPost, "/api/skills", strings.NewReader(skillBody))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upsert skill status = %d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/skills/k8s-debug", nil)
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("get skill status = %d body=%s", response.Code, response.Body.String())
	}

	ruleBody := `{"rule_id":"crashloop","name":"CrashLoop","is_active":true,"trigger_source":"prometheus_alertmanager","match_expression":{"alertname":"K8sPodCrashLooping"},"bind_skills":["k8s-debug"],"agent_image":"ballast-runner-base:dev","policy_group":"k8s_prod_write_intercept"}`
	request = httptest.NewRequest(http.MethodPost, "/api/trigger-rules", strings.NewReader(ruleBody))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upsert trigger rule status = %d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/trigger-rules/crashloop", nil)
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("get trigger rule status = %d body=%s", response.Code, response.Body.String())
	}
}

type memorySkillRepo struct {
	mu     sync.Mutex
	skills map[string]*domain.Skill
}

func newMemorySkillRepo() *memorySkillRepo {
	return &memorySkillRepo{skills: make(map[string]*domain.Skill)}
}

func (m *memorySkillRepo) Upsert(_ context.Context, skill *domain.Skill) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cloned := *skill
	m.skills[skill.SkillID] = &cloned
	return nil
}

func (m *memorySkillRepo) Get(_ context.Context, id string) (*domain.Skill, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	skill := m.skills[id]
	if skill == nil {
		return nil, store.ErrNotFound
	}
	cloned := *skill
	return &cloned, nil
}

func (m *memorySkillRepo) List(context.Context) ([]*domain.Skill, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*domain.Skill, 0, len(m.skills))
	for _, skill := range m.skills {
		cloned := *skill
		out = append(out, &cloned)
	}
	return out, nil
}

type memoryTriggerRuleRepo struct {
	mu    sync.Mutex
	rules map[string]*domain.TriggerRule
}

func newMemoryTriggerRuleRepo() *memoryTriggerRuleRepo {
	return &memoryTriggerRuleRepo{rules: make(map[string]*domain.TriggerRule)}
}

func (m *memoryTriggerRuleRepo) Upsert(_ context.Context, rule *domain.TriggerRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cloned := *rule
	m.rules[rule.RuleID] = &cloned
	return nil
}

func (m *memoryTriggerRuleRepo) Get(_ context.Context, id string) (*domain.TriggerRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rule := m.rules[id]
	if rule == nil {
		return nil, store.ErrNotFound
	}
	cloned := *rule
	return &cloned, nil
}

func (m *memoryTriggerRuleRepo) List(context.Context) ([]*domain.TriggerRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*domain.TriggerRule, 0, len(m.rules))
	for _, rule := range m.rules {
		cloned := *rule
		out = append(out, &cloned)
	}
	return out, nil
}
