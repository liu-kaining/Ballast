// Package api 实现 Ballast 控制面的 REST + WebSocket API。
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/orchestrator"
	"github.com/ballast/ballast-server/internal/policy"
)

// Router 聚合 API 依赖。
type Router struct {
	mgr    *orchestrator.Manager
	policy policy.PolicyEngine
	logger *log.Logger
}

// NewRouter 构造 Router 并注册路由到 mux。
func NewRouter(mux *http.ServeMux, mgr *orchestrator.Manager, pol policy.PolicyEngine, logger *log.Logger) *Router {
	r := &Router{mgr: mgr, policy: pol, logger: logger}
	mux.HandleFunc("/api/sessions", r.handleSessions)
	mux.HandleFunc("/api/sessions/", r.handleSessionItem)
	mux.HandleFunc("/api/internal/harness/report", r.handleHarnessReport)
	return r
}

// ---- /api/sessions (GET 列表, POST 创建) ----

func (r *Router) handleSessions(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.listSessions(w, req)
	case http.MethodPost:
		r.createSession(w, req)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (r *Router) listSessions(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
	status := domain.SessionStatus(req.URL.Query().Get("status"))
	list, err := r.mgr.ListSessions(req.Context(), status, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

type createSessionReq struct {
	Title      string `json:"title"`
	AgentImage string `json:"agent_image"`
}

func (r *Router) createSession(w http.ResponseWriter, req *http.Request) {
	var body createSessionReq
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Title == "" {
		body.Title = "未命名会话"
	}
	if body.AgentImage == "" {
		body.AgentImage = "ballast-runner-base:dev"
	}
	sess, err := r.mgr.CreateSession(req.Context(), body.Title, body.AgentImage)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// ---- /api/sessions/{id} (GET 详情) 与子动作 /approve /resume /destroy + WS /ws ----

func (r *Router) handleSessionItem(w http.ResponseWriter, req *http.Request) {
	// 路径：/api/sessions/{id}  或  /api/sessions/{id}/{action}  或  /api/sessions/{id}/ws
	path := req.URL.Path[len("/api/sessions/"):]
	if path == "" {
		http.NotFound(w, req)
		return
	}
	id, rest := splitFirst(path)
	switch {
	case rest == "" && req.Method == http.MethodGet:
		r.getSession(w, req, id)
	case rest == "approve" && req.Method == http.MethodPost:
		r.approveSession(w, req, id)
	case rest == "resume" && req.Method == http.MethodPost:
		r.resumeSession(w, req, id)
	case rest == "destroy" && req.Method == http.MethodPost:
		r.destroySession(w, req, id)
	case rest == "ws":
		r.serveWS(w, req, id)
	default:
		http.NotFound(w, req)
	}
}

func (r *Router) getSession(w http.ResponseWriter, req *http.Request, id string) {
	sess, err := r.mgr.GetSession(req.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

type approveReq struct {
	Approver string `json:"approver"`
}

func (r *Router) approveSession(w http.ResponseWriter, req *http.Request, id string) {
	var body approveReq
	_ = json.NewDecoder(req.Body).Decode(&body)
	if body.Approver == "" {
		body.Approver = "anonymous"
	}
	if err := r.mgr.Approve(req.Context(), id, body.Approver); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (r *Router) resumeSession(w http.ResponseWriter, req *http.Request, id string) {
	// v0.1 resume 等价于 approve
	r.approveSession(w, req, id)
}

func (r *Router) destroySession(w http.ResponseWriter, req *http.Request, id string) {
	if err := r.mgr.Destroy(req.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "destroyed"})
}

// ---- WebSocket /api/sessions/{id}/ws ----

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (r *Router) serveWS(w http.ResponseWriter, req *http.Request, id string) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		r.logger.Printf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	ch := r.mgr.Subscribe(id)
	defer r.mgr.Unsubscribe(id, ch)

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	// 读循环：忽略客户端消息（v0.1 终端只读流）；检测断开
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		}
	}
}

// ---- /api/internal/harness/report（沙箱内 harness-agent guard 上报） ----

func (r *Router) handleHarnessReport(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var cmdCtx policy.CommandContext
	if err := json.NewDecoder(req.Body).Decode(&cmdCtx); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	dec, err := r.policy.EvaluateCommand(req.Context(), cmdCtx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"decision": string(dec), "reason": ""})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func splitFirst(s string) (first, rest string) {
	for i, r := range s {
		if r == '/' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
