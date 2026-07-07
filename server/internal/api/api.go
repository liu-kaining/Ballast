// Package api implements the authenticated REST, internal harness and
// WebSocket interfaces of the Ballast control plane.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/orchestrator"
	"github.com/ballast/ballast-server/internal/policy"
	"github.com/ballast/ballast-server/internal/store"
)

const (
	sessionCookieName = "ballast_session"
	maxRequestBody    = 1 << 20
)

type Options struct {
	AdminToken         string
	SessionSecret      string
	InternalToken      string
	CORSAllowedOrigins []string
	CookieSecure       bool
	Skills             SkillRepository
	TriggerRules       TriggerRuleRepository
}

type SkillRepository interface {
	Upsert(context.Context, *domain.Skill) error
	Get(context.Context, string) (*domain.Skill, error)
	List(context.Context) ([]*domain.Skill, error)
}

type TriggerRuleRepository interface {
	Upsert(context.Context, *domain.TriggerRule) error
	Get(context.Context, string) (*domain.TriggerRule, error)
	List(context.Context) ([]*domain.TriggerRule, error)
}

type Router struct {
	manager        *orchestrator.Manager
	logger         *log.Logger
	options        Options
	allowedOrigins map[string]struct{}
	upgrader       websocket.Upgrader
}

func NewRouter(mux *http.ServeMux, manager *orchestrator.Manager, logger *log.Logger, options Options) *Router {
	r := &Router{
		manager:        manager,
		logger:         logger,
		options:        options,
		allowedOrigins: make(map[string]struct{}),
	}
	for _, origin := range options.CORSAllowedOrigins {
		if origin = strings.TrimSpace(origin); origin != "" {
			r.allowedOrigins[origin] = struct{}{}
		}
	}
	r.upgrader.CheckOrigin = r.originAllowed

	mux.HandleFunc("/api/auth/login", r.withCORS(r.handleLogin))
	mux.HandleFunc("/api/auth/logout", r.withCORS(r.handleLogout))
	mux.HandleFunc("/api/sessions", r.withCORS(r.requireSession(r.handleSessions)))
	mux.HandleFunc("/api/sessions/", r.withCORS(r.requireSession(r.handleSessionItem)))
	mux.HandleFunc("/api/skills", r.withCORS(r.requireSession(r.handleSkills)))
	mux.HandleFunc("/api/skills/", r.withCORS(r.requireSession(r.handleSkillItem)))
	mux.HandleFunc("/api/trigger-rules", r.withCORS(r.requireSession(r.handleTriggerRules)))
	mux.HandleFunc("/api/trigger-rules/", r.withCORS(r.requireSession(r.handleTriggerRuleItem)))
	mux.HandleFunc("/api/internal/harness/report", r.requireInternal(r.handleHarnessReport))
	mux.HandleFunc("/api/internal/harness/event", r.requireInternal(r.handleHarnessEvent))
	return r
}

func (r *Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, req, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !secureEqual(body.Token, r.options.AdminToken) {
		writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
		return
	}

	const ttl = 12 * time.Hour
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    signSession(time.Now().Add(ttl), r.options.SessionSecret),
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   r.options.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
}

func (r *Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.options.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (r *Router) handleSessions(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch req.Method {
	case http.MethodGet:
		r.listSessions(w, req)
	case http.MethodPost:
		r.createSession(w, req)
	default:
		methodNotAllowed(w)
	}
}

func (r *Router) listSessions(w http.ResponseWriter, req *http.Request) {
	limit, err := parseNonNegativeInt(req.URL.Query().Get("limit"), 50)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid limit"))
		return
	}
	if limit > 100 {
		limit = 100
	}
	offset, err := parseNonNegativeInt(req.URL.Query().Get("offset"), 0)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid offset"))
		return
	}
	status := domain.SessionStatus(req.URL.Query().Get("status"))
	if status != "" && !validStatus(status) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid status %q", status))
		return
	}

	sessions, err := r.manager.ListSessions(req.Context(), status, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if sessions == nil {
		sessions = []*domain.Session{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (r *Router) createSession(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Title      string   `json:"title"`
		AgentImage string   `json:"agent_image"`
		SkillIDs   []string `json:"skill_ids"`
	}
	if err := decodeJSON(w, req, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		body.Title = "未命名会话"
	}
	if len([]rune(body.Title)) > 255 {
		writeErr(w, http.StatusBadRequest, errors.New("title exceeds 255 characters"))
		return
	}
	session, err := r.manager.CreateSession(req.Context(), body.Title, body.AgentImage, body.SkillIDs...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (r *Router) handleSessionItem(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	path := strings.TrimPrefix(req.URL.Path, "/api/sessions/")
	id, rest := splitFirst(path)
	if id == "" {
		http.NotFound(w, req)
		return
	}
	switch {
	case rest == "" && req.Method == http.MethodGet:
		r.getSession(w, req, id)
	case rest == "approve" && req.Method == http.MethodPost:
		r.approveSession(w, req, id)
	case rest == "resume" && req.Method == http.MethodPost:
		r.approveSession(w, req, id)
	case rest == "audit" && req.Method == http.MethodGet:
		r.listAudit(w, req, id)
	case rest == "destroy" && req.Method == http.MethodPost:
		r.destroySession(w, req, id)
	case rest == "ws" && req.Method == http.MethodGet:
		r.serveWS(w, req, id)
	default:
		http.NotFound(w, req)
	}
}

func (r *Router) getSession(w http.ResponseWriter, req *http.Request, id string) {
	session, err := r.manager.GetSession(req.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (r *Router) approveSession(w http.ResponseWriter, req *http.Request, id string) {
	approver := "operator"
	if req.ContentLength != 0 {
		var body struct {
			Approver string `json:"approver"`
		}
		if err := decodeJSON(w, req, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if body.Approver = strings.TrimSpace(body.Approver); body.Approver != "" {
			approver = body.Approver
		}
	}
	if len([]rune(approver)) > 64 {
		writeErr(w, http.StatusBadRequest, errors.New("approver exceeds 64 characters"))
		return
	}
	if err := r.manager.Approve(req.Context(), id, approver); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (r *Router) listAudit(w http.ResponseWriter, req *http.Request, id string) {
	if _, err := r.manager.GetSession(req.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	limit, err := parseNonNegativeInt(req.URL.Query().Get("limit"), 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid limit"))
		return
	}
	logs, err := r.manager.ListAudit(req.Context(), id, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if logs == nil {
		logs = []*domain.AuditLog{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit_logs": logs})
}

func (r *Router) destroySession(w http.ResponseWriter, req *http.Request, id string) {
	if err := r.manager.Destroy(req.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "destroyed"})
}

func (r *Router) serveWS(w http.ResponseWriter, req *http.Request, id string) {
	if _, err := r.manager.GetSession(req.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	connection, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		r.logger.Printf("ws upgrade: %v", err)
		return
	}
	defer connection.Close()

	events := r.manager.Subscribe(id)
	defer r.manager.Unsubscribe(id, events)

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	connection.SetReadLimit(4096)
	_ = connection.SetReadDeadline(time.Now().Add(60 * time.Second))
	connection.SetPongHandler(func(string) error {
		return connection.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	go func() {
		for {
			if _, _, err := connection.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if err := connection.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := connection.WriteJSON(event); err != nil {
				return
			}
		}
	}
}

func (r *Router) handleHarnessReport(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var command policy.CommandContext
	if err := decodeJSON(w, req, &command); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	decision, err := r.manager.EvaluateCommand(req.Context(), command)
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"decision": string(decision),
		"reason":   "",
	})
}

func (r *Router) handleHarnessEvent(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var event struct {
		SessionID string          `json:"session_id"`
		Type      string          `json:"type"`
		Data      json.RawMessage `json:"data"`
	}
	if err := decodeJSON(w, req, &event); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := r.manager.HandleHarnessEvent(req.Context(), event.SessionID, event.Type, event.Data); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (r *Router) handleSkills(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.options.Skills == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("skill repository is not configured"))
		return
	}
	switch req.Method {
	case http.MethodGet:
		skills, err := r.options.Skills.List(req.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if skills == nil {
			skills = []*domain.Skill{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
	case http.MethodPost:
		var skill domain.Skill
		if err := decodeJSON(w, req, &skill); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		normalized, err := normalizeSkill(&skill)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := r.options.Skills.Upsert(req.Context(), normalized); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, normalized)
	default:
		methodNotAllowed(w)
	}
}

func (r *Router) handleSkillItem(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.options.Skills == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("skill repository is not configured"))
		return
	}
	id := strings.TrimPrefix(req.URL.Path, "/api/skills/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, req)
		return
	}
	if !assetIDPattern.MatchString(id) {
		writeErr(w, http.StatusBadRequest, errors.New("invalid skill id"))
		return
	}
	if req.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	skill, err := r.options.Skills.Get(req.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, skill)
}

func (r *Router) handleTriggerRules(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.options.TriggerRules == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("trigger rule repository is not configured"))
		return
	}
	switch req.Method {
	case http.MethodGet:
		rules, err := r.options.TriggerRules.List(req.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if rules == nil {
			rules = []*domain.TriggerRule{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"trigger_rules": rules})
	case http.MethodPost:
		var rule domain.TriggerRule
		if err := decodeJSON(w, req, &rule); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		normalized, err := normalizeTriggerRule(&rule)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := r.options.TriggerRules.Upsert(req.Context(), normalized); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, normalized)
	default:
		methodNotAllowed(w)
	}
}

func (r *Router) handleTriggerRuleItem(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.options.TriggerRules == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("trigger rule repository is not configured"))
		return
	}
	id := strings.TrimPrefix(req.URL.Path, "/api/trigger-rules/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, req)
		return
	}
	if !assetIDPattern.MatchString(id) {
		writeErr(w, http.StatusBadRequest, errors.New("invalid trigger rule id"))
		return
	}
	if req.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	rule, err := r.options.TriggerRules.Get(req.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (r *Router) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodOptions {
			next(w, req)
			return
		}
		cookie, err := req.Cookie(sessionCookieName)
		if err != nil || !verifySession(cookie.Value, r.options.SessionSecret, time.Now()) {
			writeErr(w, http.StatusUnauthorized, errors.New("authentication required"))
			return
		}
		next(w, req)
	}
}

func (r *Router) requireInternal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		const prefix = "Bearer "
		header := req.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) || !secureEqual(strings.TrimPrefix(header, prefix), r.options.InternalToken) {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid internal credential"))
			return
		}
		next(w, req)
	}
}

func (r *Router) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		origin := req.Header.Get("Origin")
		if origin != "" {
			if !r.originAllowed(req) {
				writeErr(w, http.StatusForbidden, errors.New("origin is not allowed"))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Add("Vary", "Origin")
		}
		next(w, req)
	}
}

func (r *Router) originAllowed(req *http.Request) bool {
	origin := req.Header.Get("Origin")
	if origin == "" {
		return true
	}
	_, ok := r.allowedOrigins[origin]
	return ok
}

func signSession(expires time.Time, secret string) string {
	payload := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifySession(value, secret string, now time.Time) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return false
	}
	expires, err := strconv.ParseInt(string(payload), 10, 64)
	return err == nil && now.Unix() < expires
}

func secureEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func decodeJSON(w http.ResponseWriter, req *http.Request, target any) error {
	req.Body = http.MaxBytesReader(w, req.Body, maxRequestBody)
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func splitFirst(value string) (string, string) {
	if index := strings.IndexByte(value, '/'); index >= 0 {
		return value[:index], value[index+1:]
	}
	return value, ""
}

func parseNonNegativeInt(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, errors.New("not a non-negative integer")
	}
	return value, nil
}

func validStatus(status domain.SessionStatus) bool {
	switch status {
	case domain.SessionRunning, domain.SessionSuspended, domain.SessionSuccess, domain.SessionFailed:
		return true
	default:
		return false
	}
}

var assetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func normalizeSkill(skill *domain.Skill) (*domain.Skill, error) {
	skill.SkillID = strings.TrimSpace(skill.SkillID)
	skill.Name = strings.TrimSpace(skill.Name)
	skill.Description = strings.TrimSpace(skill.Description)
	skill.MarkdownContent = strings.TrimSpace(skill.MarkdownContent)
	skill.UpdatedBy = strings.TrimSpace(skill.UpdatedBy)
	if !assetIDPattern.MatchString(skill.SkillID) {
		return nil, fmt.Errorf("invalid skill_id %q", skill.SkillID)
	}
	if skill.Name == "" {
		return nil, errors.New("name is required")
	}
	if len([]rune(skill.Name)) > 128 {
		return nil, errors.New("name exceeds 128 characters")
	}
	if skill.MarkdownContent == "" {
		return nil, errors.New("markdown_content is required")
	}
	if skill.UpdatedBy == "" {
		skill.UpdatedBy = "operator"
	}
	if len([]rune(skill.UpdatedBy)) > 64 {
		return nil, errors.New("updated_by exceeds 64 characters")
	}
	if skill.Version <= 0 {
		skill.Version = 1
	}
	skill.TriggerWords = cleanStringList(skill.TriggerWords, 32)
	return skill, nil
}

func normalizeTriggerRule(rule *domain.TriggerRule) (*domain.TriggerRule, error) {
	rule.RuleID = strings.TrimSpace(rule.RuleID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.TriggerSource = strings.TrimSpace(rule.TriggerSource)
	rule.AgentImage = strings.TrimSpace(rule.AgentImage)
	rule.PolicyGroup = strings.TrimSpace(rule.PolicyGroup)
	if !assetIDPattern.MatchString(rule.RuleID) {
		return nil, fmt.Errorf("invalid rule_id %q", rule.RuleID)
	}
	if rule.Name == "" {
		return nil, errors.New("name is required")
	}
	if len([]rune(rule.Name)) > 128 {
		return nil, errors.New("name exceeds 128 characters")
	}
	if rule.TriggerSource == "" {
		return nil, errors.New("trigger_source is required")
	}
	if rule.AgentImage == "" {
		return nil, errors.New("agent_image is required")
	}
	if rule.PolicyGroup == "" {
		return nil, errors.New("policy_group is required")
	}
	if len(rule.MatchExpression) == 0 || !json.Valid(rule.MatchExpression) {
		return nil, errors.New("match_expression must be valid JSON")
	}
	rule.BindSkills = cleanStringList(rule.BindSkills, 64)
	for _, skillID := range rule.BindSkills {
		if !assetIDPattern.MatchString(skillID) {
			return nil, fmt.Errorf("invalid bind skill id %q", skillID)
		}
	}
	return rule, nil
}

func cleanStringList(values []string, max int) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= max {
			break
		}
	}
	return out
}
