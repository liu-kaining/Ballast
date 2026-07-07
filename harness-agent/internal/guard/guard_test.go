package guard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseEventAndCommand(t *testing.T) {
	line := `{"type":"tool.call","properties":{"tool":"bash","command":"kubectl apply -f fixed.yaml"}}`
	event, ok := ParseEvent(line)
	if !ok || event.Type != "tool.call" || !json.Valid(event.Data) {
		t.Fatalf("event = %#v ok=%v", event, ok)
	}
	if command := ParseCommand(line); command != "kubectl apply -f fixed.yaml" {
		t.Fatalf("command = %q", command)
	}
	executable, args := SplitCommand(ParseCommand(line))
	if executable != "kubectl" || len(args) != 3 || args[0] != "apply" {
		t.Fatalf("split = %q %#v", executable, args)
	}
}

func TestHTTPReporterSendsInternalCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(ReportResponse{Decision: Approve})
	}))
	defer server.Close()

	reporter := NewHTTPReporter(server.URL, "agent", "secret")
	response, err := reporter.Report(context.Background(), CommandContext{Command: "kubectl"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Decision != Approve {
		t.Fatalf("decision = %s", response.Decision)
	}
}

func TestAnalyzeCommandRejectsShellComposition(t *testing.T) {
	command, args, unsafe := AnalyzeCommand("kubectl get pods && rm -rf /")
	if command != "kubectl" || len(args) == 0 || !unsafe {
		t.Fatalf("analysis = %q %#v unsafe=%v", command, args, unsafe)
	}
	_, _, unsafe = AnalyzeCommand("kubectl get pods -n prod")
	if unsafe {
		t.Fatal("simple read-only command marked unsafe")
	}
}

func TestRedactEnvironment(t *testing.T) {
	got := RedactEnvironment([]string{
		"PATH=/usr/bin",
		"BALLAST_INTERNAL_TOKEN=secret",
		"API_KEY=key",
	})
	if got[0] != "PATH=/usr/bin" ||
		got[1] != "BALLAST_INTERNAL_TOKEN=<redacted>" ||
		got[2] != "API_KEY=<redacted>" {
		t.Fatalf("redacted environment = %#v", got)
	}
}
