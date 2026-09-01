//go:build windows

package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileExactAndIntegrityCheck(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source.exe")
	target := filepath.Join(directory, "target.rollback")
	payload := []byte("remoteit-update-rollback-test")
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFileExact(source, target); err != nil {
		t.Fatal(err)
	}
	if err := verifyIdenticalFiles(source, target); err != nil {
		t.Fatalf("verified rollback copy rejected: %v", err)
	}
	if err := os.WriteFile(target, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyIdenticalFiles(source, target); err == nil {
		t.Fatal("corrupt rollback copy was accepted")
	}
}

func TestRollbackCopyPreservesBoundInstallerPayload(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "bound-installer.exe")
	exactTarget := filepath.Join(directory, "bound-installer.rollback")
	installedTarget := filepath.Join(directory, "installed-agent.exe")
	base := []byte("MZ-remoteit-agent-base")
	payload := []byte(`{"token":"one-time-enrollment-token","serverUrl":"https://example.test"}`)
	bound := append(append(append([]byte{}, base...), payload...), []byte(boundInstallerMagic)...)
	length := make([]byte, 4)
	binary.LittleEndian.PutUint32(length, uint32(len(payload)))
	bound = append(bound, length...)
	if err := os.WriteFile(source, bound, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := copyFileExact(source, exactTarget); err != nil {
		t.Fatal(err)
	}
	if err := verifyIdenticalFiles(source, exactTarget); err != nil {
		t.Fatalf("rollback lost the bound payload: %v", err)
	}

	if err := copyFile(source, installedTarget); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(installedTarget)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(base) {
		t.Fatalf("normal installation did not strip enrollment payload: %q", installed)
	}
}

func TestCleanupStaleAgentFilesPreservesRollback(t *testing.T) {
	directory := t.TempDir()
	active := filepath.Join(directory, "RemoteIt-Agent.exe")
	rollback := active + windowsUpdateRollbackSuffix
	stale := filepath.Join(directory, "RemoteIt-Agent-1.0.36.old")
	for _, path := range []string{active, rollback, stale} {
		if err := os.WriteFile(path, []byte("agent"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cleanupStaleAgentFiles(directory, active)
	for _, path := range []string{active, rollback} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected update file removed: %s: %v", path, err)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale update file was not removed: %v", err)
	}
}
