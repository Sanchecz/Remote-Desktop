package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamBoundMacOSAgentBuildsUniversalApplicationBundle(t *testing.T) {
	root := t.TempDir()
	downloads := filepath.Join(root, "downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"remoteit-agent-macos-arm64", "remoteit-agent-macos-amd64"} {
		if err := os.WriteFile(filepath.Join(downloads, name), bytes.Repeat([]byte(name), 160), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := &server{webRoot: root, publicURL: "https://remoteit.example"}
	recorder := httptest.NewRecorder()
	if !s.streamBoundMacOSAgent(recorder, "token-with-'quote") {
		t.Fatal("macOS bundle was not streamed")
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type=%q", got)
	}
	archive, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	if err != nil {
		t.Fatalf("invalid ZIP: %v", err)
	}
	entries := map[string]*zip.File{}
	for _, entry := range archive.File {
		entries[entry.Name] = entry
	}
	for _, name := range []string{
		"RemoteIt Agent.app/Contents/Info.plist",
		"RemoteIt Agent.app/Contents/MacOS/RemoteIt Agent",
		"RemoteIt Agent.app/Contents/Resources/remoteit-agent-macos-arm64",
		"RemoteIt Agent.app/Contents/Resources/remoteit-agent-macos-amd64",
		"УСТАНОВКА.txt",
	} {
		if entries[name] == nil {
			t.Fatalf("bundle entry missing: %s", name)
		}
	}
	launcherEntry := entries["RemoteIt Agent.app/Contents/MacOS/RemoteIt Agent"]
	if launcherEntry.Mode().Perm() != 0o755 {
		t.Fatalf("launcher mode=%o", launcherEntry.Mode().Perm())
	}
	reader, err := launcherEntry.Open()
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"token-with-'\"'\"'quote", "https://remoteit.example", "with administrator privileges", "remoteit-agent-macos-arm64", "remoteit-agent-macos-amd64"} {
		if !strings.Contains(string(launcher), expected) {
			t.Fatalf("launcher does not contain %q", expected)
		}
	}
}

func TestStreamBoundMacOSAgentFailsBeforeWritingWhenArtifactIsMissing(t *testing.T) {
	recorder := httptest.NewRecorder()
	s := &server{webRoot: t.TempDir(), publicURL: "https://remoteit.example"}
	if s.streamBoundMacOSAgent(recorder, "token") {
		t.Fatal("bundle unexpectedly succeeded")
	}
	if recorder.Code != 503 {
		t.Fatalf("status=%d", recorder.Code)
	}
}
