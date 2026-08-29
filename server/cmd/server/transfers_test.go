package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func transferRequest(body string) *http.Request {
	return httptest.NewRequest("PUT", "/api/file-transfers/test/data", strings.NewReader(body))
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
