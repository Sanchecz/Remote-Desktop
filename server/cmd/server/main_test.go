package main

import (
	"strings"
	"testing"
)

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
