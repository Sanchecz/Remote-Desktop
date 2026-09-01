package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTimeoutUnlessWebSocket(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		headers   http.Header
		wantTimed bool
	}{
		{name: "ordinary request", method: http.MethodGet, path: "/api/devices", wantTimed: true},
		{name: "viewer websocket upgrade", method: http.MethodGet, path: "/api/desktop-sessions/session-id/stream", headers: http.Header{"Upgrade": {"websocket"}, "Connection": {"keep-alive, Upgrade"}}, wantTimed: false},
		{name: "agent websocket upgrade", method: http.MethodGet, path: "/api/desktop/agent/sessions/session-id/stream", headers: http.Header{"Upgrade": {"websocket"}, "Connection": {"Upgrade"}}, wantTimed: false},
		{name: "forged upgrade on ordinary API", method: http.MethodGet, path: "/api/devices", headers: http.Header{"Upgrade": {"websocket"}, "Connection": {"Upgrade"}}, wantTimed: true},
		{name: "post cannot bypass timeout", method: http.MethodPost, path: "/api/desktop-sessions/session-id/stream", headers: http.Header{"Upgrade": {"websocket"}, "Connection": {"Upgrade"}}, wantTimed: true},
		{name: "upgrade header without connection token", method: http.MethodGet, path: "/api/desktop-sessions/session-id/stream", headers: http.Header{"Upgrade": {"websocket"}}, wantTimed: true},
		{name: "connection token without websocket upgrade", method: http.MethodGet, path: "/api/desktop-sessions/session-id/stream", headers: http.Header{"Connection": {"Upgrade"}}, wantTimed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := timeoutUnlessWebSocket(time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, hasDeadline := r.Context().Deadline()
				if hasDeadline != test.wantTimed {
					t.Fatalf("deadline present = %v, want %v", hasDeadline, test.wantTimed)
				}
				if test.wantTimed && r.Context().Value(deadlineProbeKey{}) != "kept" {
					t.Fatal("request context values were not preserved")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(test.method, test.path, nil)
			request = request.WithContext(context.WithValue(request.Context(), deadlineProbeKey{}, "kept"))
			request.Header = test.headers.Clone()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
			}
		})
	}
}

type deadlineProbeKey struct{}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword("correct horse battery staple", hash) {
		t.Fatal("valid password was rejected")
	}
	if verifyPassword("wrong password", hash) {
		t.Fatal("invalid password was accepted")
	}
}

func TestInputSanitizers(t *testing.T) {
	ips := sanitizeIPs([]string{"127.0.0.1", "invalid", "2001:db8::1"})
	if len(ips) != 2 || ips[0] != "127.0.0.1" || ips[1] != "2001:db8::1" {
		t.Fatalf("unexpected IPs: %#v", ips)
	}
	if sanitizeInstallMode("system") != "system" || sanitizeInstallMode("other") != "unknown" {
		t.Fatal("unexpected install mode sanitization")
	}
	if truncate("  abcdef  ", 3) != "abc" {
		t.Fatal("unexpected truncation")
	}
}

func TestConnectionCodeShape(t *testing.T) {
	code := connectionCode()
	if len(code) != 9 || code[0] == '0' {
		t.Fatalf("invalid connection code: %q", code)
	}
}

func TestSemanticVersionAtLeast(t *testing.T) {
	tests := []struct {
		actual, required string
		want             bool
	}{
		{"0.6.0", "0.6.0", true},
		{"0.6.1", "0.6.0", true},
		{"1.0.0", "0.6.0", true},
		{"0.5.9", "0.6.0", false},
		{"", "0.6.0", false},
		{"v0.6.0-beta", "0.6.0", true},
	}
	for _, test := range tests {
		if got := semanticVersionAtLeast(test.actual, test.required); got != test.want {
			t.Fatalf("semanticVersionAtLeast(%q, %q) = %v, want %v", test.actual, test.required, got, test.want)
		}
	}
}

func TestShellForDeviceOS(t *testing.T) {
	tests := []struct {
		name      string
		deviceOS  string
		requested string
		want      string
		wantError bool
	}{
		{name: "windows default", deviceOS: "Windows", want: "powershell"},
		{name: "windows cmd", deviceOS: "Microsoft Windows", requested: "CMD", want: "cmd"},
		{name: "linux default", deviceOS: "Ubuntu", want: "bash"},
		{name: "linux bash", deviceOS: "Linux", requested: "bash", want: "bash"},
		{name: "mac default", deviceOS: "macOS", want: "zsh"},
		{name: "mac bash", deviceOS: "Darwin", requested: "bash", want: "bash"},
		{name: "powershell rejected on linux", deviceOS: "Ubuntu", requested: "powershell", wantError: true},
		{name: "zsh rejected on windows", deviceOS: "Windows", requested: "zsh", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := shellForDeviceOS(test.deviceOS, test.requested)
			if test.wantError {
				if err == nil {
					t.Fatalf("shellForDeviceOS(%q, %q) = %q, want error", test.deviceOS, test.requested, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("shellForDeviceOS(%q, %q) = %q, %v; want %q", test.deviceOS, test.requested, got, err, test.want)
			}
		})
	}
}

func TestAgentReleaseArtifactValidation(t *testing.T) {
	valid := agentReleaseArtifact{Path: "/downloads/remoteit-agent-windows-amd64.exe", SHA256: strings.Repeat("a", 64), Size: 1024}
	if !validAgentReleaseArtifact(valid) {
		t.Fatal("valid release artifact was rejected")
	}
	for _, artifact := range []agentReleaseArtifact{
		{Path: "/other/agent.exe", SHA256: valid.SHA256, Size: valid.Size},
		{Path: "/downloads/../secret", SHA256: valid.SHA256, Size: valid.Size},
		{Path: valid.Path, SHA256: "bad", Size: valid.Size},
		{Path: valid.Path, SHA256: valid.SHA256, Size: 0},
	} {
		if validAgentReleaseArtifact(artifact) {
			t.Fatalf("invalid artifact was accepted: %#v", artifact)
		}
	}
}

func TestNormalizedAgentPlatform(t *testing.T) {
	checks := map[string]string{
		normalizedAgentPlatform("Windows", "amd64"):      "windows-amd64",
		normalizedAgentPlatform("Ubuntu 26.04", "amd64"): "linux-amd64",
		normalizedAgentPlatform("macOS", "arm64"):        "darwin-arm64",
		normalizedAgentPlatform("Windows", "386"):        "",
	}
	for actual, expected := range checks {
		if actual != expected {
			t.Fatalf("unexpected platform %q, expected %q", actual, expected)
		}
	}
}
