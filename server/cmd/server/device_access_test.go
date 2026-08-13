package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidDeviceAccessPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "too short", password: "1234567", valid: false},
		{name: "minimum", password: "12345678", valid: true},
		{name: "unicode counted as runes", password: "пароль-1", valid: true},
		{name: "maximum", password: strings.Repeat("a", 128), valid: true},
		{name: "too long", password: strings.Repeat("a", 129), valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := validDeviceAccessPassword(test.password); actual != test.valid {
				t.Fatalf("validDeviceAccessPassword() = %v, want %v", actual, test.valid)
			}
		})
	}
}

func TestDeviceUnlockAttemptKeyIsScoped(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/devices/device-a/unlock", nil)
	request.Header.Set("X-Forwarded-For", "192.0.2.10")
	auth := &authState{UserID: "user-a"}
	first := deviceUnlockAttemptKey(request, auth, "device-a")
	second := deviceUnlockAttemptKey(request, auth, "device-b")
	if first == second {
		t.Fatal("unlock rate limit keys must be scoped to a device")
	}
	if !strings.Contains(first, "192.0.2.10") || !strings.Contains(first, "user-a") {
		t.Fatalf("unlock rate limit key is missing request or account scope: %q", first)
	}
}

func TestDeviceAccessPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPassword("strong-device-password")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if !verifyPassword("strong-device-password", hash) {
		t.Fatal("correct device password did not verify")
	}
	if verifyPassword("wrong-device-password", hash) {
		t.Fatal("incorrect device password verified")
	}
}
