package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
