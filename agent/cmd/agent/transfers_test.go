package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestTransferActivityReaderRefreshesProgress(t *testing.T) {
	touches := 0
	reader := transferActivityReader{reader: bytes.NewBufferString("remoteit"), touch: func() { touches++ }}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "remoteit" {
		t.Fatalf("ReadAll()=(%q,%v)", data, err)
	}
	if touches == 0 {
		t.Fatal("activity reader did not refresh the idle deadline")
	}
}

func TestTransferIdleWatchdogTracksActivityThenCancelsStall(t *testing.T) {
	ctx, cancel, touch := newTransferIdleWatchdog(context.Background(), 100*time.Millisecond)
	defer cancel()
	time.Sleep(55 * time.Millisecond)
	touch()
	select {
	case <-ctx.Done():
		t.Fatal("watchdog cancelled a progressing transfer")
	case <-time.After(70 * time.Millisecond):
	}
	select {
	case <-ctx.Done():
	case <-time.After(120 * time.Millisecond):
		t.Fatal("watchdog did not cancel an idle transfer")
	}
}

func TestFileTransferHTTPClientBoundsHandshakeAndHeaderWait(t *testing.T) {
	client := newFileTransferHTTPClient()
	defer client.CloseIdleConnections()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.ResponseHeaderTimeout != 30*time.Second || transport.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("unexpected transfer timeouts: header=%v tls=%v", transport.ResponseHeaderTimeout, transport.TLSHandshakeTimeout)
	}
}

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

func TestValidLargeTransferID(t *testing.T) {
	if !validLargeTransferID("01234567-89ab-cdef-0123-456789abcdef") {
		t.Fatal("valid transfer UUID was rejected")
	}
	for _, id := range []string{"", "short", "0123456789ab-cdef-0123-456789abcdef", "01234567-89AB-cdef-0123-456789abcdef", "01234567-89ab-cdef-0123-456789abcdeg"} {
		if validLargeTransferID(id) {
			t.Fatalf("invalid transfer ID %q was accepted", id)
		}
	}
}

func TestFileTransferDownloadRetry(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		failures       int
		retry          bool
		delay          time.Duration
		waitsForSource bool
	}{
		{"source not ready", http.StatusTooEarly, 4, true, 250 * time.Millisecond, true},
		{"network error", 0, 0, true, time.Second, false},
		{"rate limited", http.StatusTooManyRequests, 2, true, 3 * time.Second, false},
		{"server error capped", http.StatusBadGateway, 8, true, 5 * time.Second, false},
		{"unauthorized", http.StatusUnauthorized, 0, false, 0, false},
		{"not found", http.StatusNotFound, 0, false, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			retry, delay, waitsForSource := fileTransferDownloadRetry(test.status, test.failures)
			if retry != test.retry || delay != test.delay || waitsForSource != test.waitsForSource {
				t.Fatalf("fileTransferDownloadRetry()=(%v,%v,%v), want (%v,%v,%v)", retry, delay, waitsForSource, test.retry, test.delay, test.waitsForSource)
			}
		})
	}
}

func TestFileTransferCompletionRetry(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		attempt int
		retry   bool
		delay   time.Duration
	}{
		{"network error", 0, 0, true, time.Second},
		{"source finalizing", http.StatusTooEarly, 1, true, 2 * time.Second},
		{"conflict", http.StatusConflict, 2, true, 4 * time.Second},
		{"server error capped", http.StatusServiceUnavailable, 9, true, 8 * time.Second},
		{"forbidden", http.StatusForbidden, 0, false, 0},
		{"bad request", http.StatusBadRequest, 0, false, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			retry, delay := fileTransferCompletionRetry(test.status, test.attempt)
			if retry != test.retry || delay != test.delay {
				t.Fatalf("fileTransferCompletionRetry()=(%v,%v), want (%v,%v)", retry, delay, test.retry, test.delay)
			}
		})
	}
}
