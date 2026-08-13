package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type agentReleaseManifest struct {
	Version   string                          `json:"version"`
	Platforms map[string]agentReleaseArtifact `json:"platforms"`
}

type agentReleaseArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type availableAgentUpdate struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

func (s *server) agentUpdateFor(osName, arch, currentVersion string) *availableAgentUpdate {
	manifestPath := filepath.Join(s.webRoot, "downloads", "AGENT-RELEASE.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}
	var manifest agentReleaseManifest
	if json.Unmarshal(data, &manifest) != nil || strings.TrimSpace(manifest.Version) == "" || semanticVersionAtLeast(currentVersion, manifest.Version) {
		return nil
	}
	platform := normalizedAgentPlatform(osName, arch)
	artifact, ok := manifest.Platforms[platform]
	if !ok || !validAgentReleaseArtifact(artifact) {
		return nil
	}
	return &availableAgentUpdate{
		Version: manifest.Version,
		URL:     s.publicURL + artifact.Path,
		SHA256:  strings.ToLower(artifact.SHA256),
		Size:    artifact.Size,
	}
}

func normalizedAgentPlatform(osName, arch string) string {
	osName = strings.ToLower(strings.TrimSpace(osName))
	arch = strings.ToLower(strings.TrimSpace(arch))
	if arch != "amd64" && arch != "arm64" {
		return ""
	}
	switch {
	case strings.Contains(osName, "windows"):
		if arch == "amd64" {
			return "windows-amd64"
		}
	case strings.Contains(osName, "macos"), strings.Contains(osName, "darwin"):
		return "darwin-" + arch
	case strings.Contains(osName, "linux"), strings.Contains(osName, "ubuntu"), strings.Contains(osName, "debian"):
		if arch == "amd64" {
			return "linux-amd64"
		}
	}
	return ""
}

func validAgentReleaseArtifact(artifact agentReleaseArtifact) bool {
	if !strings.HasPrefix(artifact.Path, "/downloads/") || strings.Contains(artifact.Path, "..") || artifact.Size <= 0 || artifact.Size > 128*1024*1024 {
		return false
	}
	digest, err := hex.DecodeString(strings.TrimSpace(artifact.SHA256))
	return err == nil && len(digest) == sha256.Size
}
