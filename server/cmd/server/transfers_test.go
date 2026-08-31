package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func transferRequest(body string) *http.Request {
	return httptest.NewRequest("PUT", "/api/file-transfers/test/data", strings.NewReader(body))
}

func TestTransferChunkLockSerializesSameTransferOnly(t *testing.T) {
	s := &server{}
	first := s.transferChunkLock("a")
	if first != s.transferChunkLock("a") {
		t.Fatal("same transfer did not reuse its serialization lock")
	}
	if first == s.transferChunkLock("b") {
		t.Fatal("unrelated transfers unexpectedly share one lock")
	}

	first.Lock()
	attempting := make(chan struct{})
	entered := make(chan struct{})
	done := make(chan struct{})
	go func(lock *sync.Mutex) {
		close(attempting)
		lock.Lock()
		close(entered)
		lock.Unlock()
		close(done)
	}(s.transferChunkLock("a"))
	<-attempting
	select {
	case <-entered:
		t.Fatal("concurrent retry entered while the committed chunk was locked")
	default:
	}
	first.Unlock()
	<-done
}

func TestAppendTransferChunkSupportsOrderedResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transfer.data")
	recorder := httptest.NewRecorder()

	next, err := appendTransferChunk(recorder, transferRequest("hello "), path, 0, 11)
	if err != nil || next != 6 {
		t.Fatalf("first chunk: next=%d err=%v", next, err)
	}
	next, err = appendTransferChunk(recorder, transferRequest("world"), path, next, 11)
	if err != nil || next != 11 {
		t.Fatalf("second chunk: next=%d err=%v", next, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "hello world" {
		t.Fatalf("unexpected transfer content %q, err=%v", content, err)
	}
}

func TestAppendTransferChunkRejectsWrongOffsetWithoutChangingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transfer.data")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	next, err := appendTransferChunk(httptest.NewRecorder(), transferRequest("second"), path, 0, 11)
	if err == nil || next != 5 {
		t.Fatalf("expected stored offset 5, got next=%d err=%v", next, err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil || string(content) != "first" {
		t.Fatalf("wrong-offset request modified the file: %q, err=%v", content, readErr)
	}
}

func TestAppendTransferChunkRollsBackDataBeyondDeclaredSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transfer.data")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	next, err := appendTransferChunk(httptest.NewRecorder(), transferRequest("defg"), path, 3, 6)
	if err == nil || next != 3 {
		t.Fatalf("expected rejected chunk at offset 3, got next=%d err=%v", next, err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil || string(content) != "abc" {
		t.Fatalf("rejected chunk was not rolled back: %q, err=%v", content, readErr)
	}
}

type failingReader struct {
	delivered bool
}

func (reader *failingReader) Read(buffer []byte) (int, error) {
	if reader.delivered {
		return 0, io.ErrUnexpectedEOF
	}
	reader.delivered = true
	copy(buffer, "partial")
	return len("partial"), nil
}

func TestAppendTransferChunkRollsBackPartialReadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transfer.data")
	request := httptest.NewRequest("PUT", "/api/file-transfers/test/data", &failingReader{})
	next, err := appendTransferChunk(httptest.NewRecorder(), request, path, 0, 64)
	if err == nil || next != 0 {
		t.Fatalf("expected read failure at offset 0, got next=%d err=%v", next, err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("cannot stat rolled-back transfer: %v", statErr)
	}
	if info.Size() != 0 {
		t.Fatalf("partial read was not rolled back: size=%d", info.Size())
	}
}

func TestRollbackTransferChunkRestoresCommittedCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transfer.data")
	if err := os.WriteFile(path, []byte("committed-uncommitted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rollbackTransferChunk(path, int64(len("committed"))); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "committed" {
		t.Fatalf("rollback did not restore checkpoint: %q, err=%v", content, err)
	}
}

func TestAvailableAgentDownloadLengthPipelinesCommittedChunks(t *testing.T) {
	tests := []struct {
		name                     string
		offset, total, committed int64
		ready                    bool
		status                   string
		wantLength               int64
		wantWait, wantError      bool
	}{
		{name: "first committed part", total: 128 << 20, committed: 32 << 20, status: "transferring", wantLength: 32 << 20},
		{name: "next committed part", offset: 32 << 20, total: 128 << 20, committed: 96 << 20, status: "transferring", wantLength: 64 << 20},
		{name: "producer still uploading", offset: 32 << 20, total: 128 << 20, committed: 32 << 20, status: "transferring", wantWait: true},
		{name: "complete source", offset: 128 << 20, total: 128 << 20, committed: 128 << 20, ready: true, status: "transferring"},
		{name: "ready but incomplete", offset: 32 << 20, total: 128 << 20, committed: 32 << 20, ready: true, status: "transferring", wantError: true},
		{name: "cancelled", total: 128 << 20, committed: 32 << 20, status: "cancelled", wantError: true},
		{name: "invalid checkpoint", offset: 129 << 20, total: 128 << 20, committed: 32 << 20, status: "transferring", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			length, wait, err := availableAgentDownloadLength(test.offset, test.total, test.committed, test.ready, test.status)
			if length != test.wantLength || wait != test.wantWait || (err != nil) != test.wantError {
				t.Fatalf("length=%d wait=%v err=%v; want length=%d wait=%v error=%v", length, wait, err, test.wantLength, test.wantWait, test.wantError)
			}
		})
	}
}

func TestTransferProgressSignalWakesImmediatelyAndCoalesces(t *testing.T) {
	s := &server{}
	signal := s.transferProgressSignal("transfer-a")
	s.signalTransferProgress("transfer-a")
	s.signalTransferProgress("transfer-a")
	select {
	case <-signal:
	default:
		t.Fatal("committed checkpoint did not wake the waiting Agent")
	}
	select {
	case <-signal:
		t.Fatal("duplicate progress notifications were not coalesced")
	default:
	}

	s.finishTransferProgressSignal("transfer-a")
	select {
	case <-signal:
	default:
		t.Fatal("terminal transfer state did not wake the existing waiter")
	}
	if replacement := s.transferProgressSignal("transfer-a"); replacement == signal {
		t.Fatal("terminal transfer signal was not removed from the registry")
	}
}

func TestTransferOfferSignalWakesAgentAndCoalesces(t *testing.T) {
	s := &server{}
	signal := s.transferOfferSignal("device-a")
	s.signalTransferOffer("device-a")
	s.signalTransferOffer("device-a")
	select {
	case <-signal:
	default:
		t.Fatal("new transfer did not wake the waiting Agent")
	}
	select {
	case <-signal:
		t.Fatal("duplicate transfer offers were not coalesced")
	default:
	}
	if signal == s.transferOfferSignal("device-b") {
		t.Fatal("unrelated devices share one transfer wake-up")
	}
}
