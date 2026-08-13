package main

import (
	"strings"
	"testing"
)

func TestValidAccountPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "too short", password: "123", valid: false},
		{name: "four plain characters", password: "1234", valid: true},
		{name: "no special character required", password: "test", valid: true},
		{name: "unicode counted as runes", password: "тест", valid: true},
		{name: "maximum", password: strings.Repeat("a", 256), valid: true},
		{name: "too long", password: strings.Repeat("a", 257), valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := validAccountPassword(test.password); actual != test.valid {
				t.Fatalf("validAccountPassword() = %v, want %v", actual, test.valid)
			}
		})
	}
}
