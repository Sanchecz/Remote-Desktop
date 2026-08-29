package main

import "testing"

func TestValidTransferCheckpoint(t *testing.T) {
	tests := []struct {
		name                           string
		received, offset, length, size int64
		valid                          bool
	}{
		{"complete chunk", 68, 4, 64, 100, true},
		{"final chunk", 100, 68, 32, 100, true},
		{"partial checkpoint", 67, 4, 64, 100, false},
		{"checkpoint beyond file", 101, 68, 33, 100, false},
		{"negative offset", 63, -1, 64, 100, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validTransferCheckpoint(test.received, test.offset, test.length, test.size); got != test.valid {
				t.Fatalf("validTransferCheckpoint()=%v, want %v", got, test.valid)
			}
		})
	}
}
