package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaddyAllowsOnlySameOriginGuacamoleFrames(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "..", "Caddyfile"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(contents)
	for _, expected := range []string{
		`@remoteit_app not path /guacamole/*`,
		`X-Frame-Options "DENY"`,
		`frame-ancestors 'none'`,
		`handle /guacamole/*`,
		`X-Frame-Options "SAMEORIGIN"`,
		`frame-ancestors 'self'`,
	} {
		if !strings.Contains(config, expected) {
			t.Fatalf("Caddy frame policy is missing %q", expected)
		}
	}
}
