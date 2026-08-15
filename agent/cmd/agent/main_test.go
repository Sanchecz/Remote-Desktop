package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNetworkSignatureStable(t *testing.T) {
	first := networkSignature()
	second := networkSignature()
	if first == "" || first != second {
		t.Fatalf("network signature is not stable: %q != %q", first, second)
	}
}

func TestWaitForNetworkChangeExpires(t *testing.T) {
	started := time.Now()
	keepRunning, changed := waitForNetworkChange(context.Background(), 20*time.Millisecond, networkSignature())
	if !keepRunning || changed || time.Since(started) < 15*time.Millisecond {
		t.Fatalf("unexpected network wait result: running=%v changed=%v elapsed=%s", keepRunning, changed, time.Since(started))
	}
}

func TestAPIClientUsesFreshBoundedConnections(t *testing.T) {
	client := newAPIClient("https://supportgenesis.ru")
	if client.http.Timeout != 12*time.Second || client.transport == nil || !client.transport.DisableKeepAlives {
		t.Fatalf("network-aware control client was not configured: %#v", client)
	}
}

func TestCappedBuffer(t *testing.T) {
	buffer := &cappedBuffer{max: 5}
	written, err := buffer.Write([]byte("123456789"))
	if err != nil || written != 9 || buffer.String() != "12345" {
		t.Fatalf("unexpected capped buffer result: written=%d value=%q err=%v", written, buffer.String(), err)
	}
}

func TestTruncateText(t *testing.T) {
	if result := truncateText("  Привет  ", 4); result != "Прив" {
		t.Fatalf("unexpected truncation: %q", result)
	}
}

func TestParseKeyValues(t *testing.T) {
	values := parseKeyValues("NAME=Ubuntu\nVERSION=24.04\nignored\n")
	if values["NAME"] != "Ubuntu" || values["VERSION"] != "24.04" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestNormalizeServerURL(t *testing.T) {
	valid, err := normalizeServerURL(" https://supportgenesis.ru/ ")
	if err != nil || valid != "https://supportgenesis.ru" {
		t.Fatalf("unexpected valid URL result: %q %v", valid, err)
	}
	for _, candidate := range []string{
		"http://supportgenesis.ru",
		"https://supportgenesis.ru/api",
		"https://user:password@supportgenesis.ru",
		"not-a-url",
	} {
		if _, err := normalizeServerURL(candidate); err == nil {
			t.Fatalf("expected URL %q to be rejected", candidate)
		}
	}
}

func TestAgentUpdateValidation(t *testing.T) {
	valid := agentUpdate{
		// Keep this deliberately ahead of every normal release so a version
		// bump cannot silently turn the positive update-validation case into
		// an equal-version rejection.
		Version: "999.0.0",
		URL:     "https://supportgenesis.ru/downloads/remoteit-agent-windows-amd64.exe",
		SHA256:  strings.Repeat("a", 64),
		Size:    8 * 1024 * 1024,
	}
	if err := validateAgentUpdate("https://supportgenesis.ru", valid); err != nil {
		t.Fatalf("valid update was rejected: %v", err)
	}
	for _, candidate := range []agentUpdate{
		{Version: version, URL: valid.URL, SHA256: valid.SHA256, Size: valid.Size},
		{Version: "0.9.29", URL: "https://example.com/downloads/agent.exe", SHA256: valid.SHA256, Size: valid.Size},
		{Version: "0.9.29", URL: "http://supportgenesis.ru/downloads/agent.exe", SHA256: valid.SHA256, Size: valid.Size},
		{Version: "0.9.29", URL: valid.URL, SHA256: "bad", Size: valid.Size},
	} {
		if err := validateAgentUpdate("https://supportgenesis.ru", candidate); err == nil {
			t.Fatalf("invalid update was accepted: %#v", candidate)
		}
	}
}

func TestAgentVersionAtLeast(t *testing.T) {
	if !agentVersionAtLeast("v0.7.1-beta", "0.7.0") || agentVersionAtLeast("0.6.9", "0.7.0") {
		t.Fatal("unexpected semantic version comparison")
	}
}

func TestAPIStatusError(t *testing.T) {
	err := (&apiStatusError{StatusCode: 401, Message: "Устройство удалено"}).Error()
	if err != "сервер: Устройство удалено" {
		t.Fatalf("unexpected API error: %q", err)
	}
	withoutMessage := (&apiStatusError{StatusCode: 503}).Error()
	if withoutMessage != "сервер вернул HTTP 503" {
		t.Fatalf("unexpected status-only API error: %q", withoutMessage)
	}
}

func TestHeartbeatRestoresLegacyRemoteIDWithoutExposingItElsewhere(t *testing.T) {
	cfg := &config{DeviceID: "device", DeviceSecret: "secret", DesktopSecret: "old"}
	changed, remoteIDChanged := applyHeartbeatIdentity(cfg, heartbeatResponse{
		ConnectionCode: " 753764976 ",
		DesktopSecret:  "new-desktop-secret",
	})
	if !changed || !remoteIDChanged || cfg.ConnectionCode != "753764976" || cfg.DesktopSecret != "new-desktop-secret" {
		t.Fatalf("heartbeat identity was not restored: changed=%v idChanged=%v cfg=%#v", changed, remoteIDChanged, cfg)
	}
	changed, remoteIDChanged = applyHeartbeatIdentity(cfg, heartbeatResponse{ConnectionCode: "753764976", DesktopSecret: "new-desktop-secret"})
	if changed || remoteIDChanged {
		t.Fatal("identical heartbeat identity must not rewrite the protected configuration")
	}
}

func TestRemoteFileListAndRead(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("remote file content")
	filePath := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	listed := executeFilesListJob(&remoteJob{Payload: map[string]any{"path": root}})
	if !listed.Success {
		t.Fatalf("listing failed: %s", listed.Error)
	}
	var listing struct {
		Entries []remoteFileEntry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(listed.Output), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 2 || !listing.Entries[0].Directory || listing.Entries[1].Name != "sample.txt" {
		t.Fatalf("unexpected listing: %#v", listing.Entries)
	}

	read := executeFilesReadJob(&remoteJob{Payload: map[string]any{"path": filePath}})
	if !read.Success {
		t.Fatalf("read failed: %s", read.Error)
	}
	var downloaded struct {
		Data string `json:"dataBase64"`
	}
	if err := json.Unmarshal([]byte(read.Output), &downloaded); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(downloaded.Data)
	if err != nil || string(decoded) != string(content) {
		t.Fatalf("unexpected downloaded data: %q %v", decoded, err)
	}
}
