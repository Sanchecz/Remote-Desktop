package main

import (
	"sort"
	"strings"
)

const (
	windowsSessionStateActive       = "active"
	windowsSessionStateConnected    = "connected"
	windowsSessionStateDisconnected = "disconnected"
)

type windowsSessionCandidate struct {
	ID       uint32
	UserSID  string
	UserName string
	State    string
}

type windowsSessionSelection struct {
	SessionID    uint32
	UserSID      string
	UserName     string
	AutoBound    bool
	Ambiguous    bool
	TargetAbsent bool
}

func mergeWindowsSessionBinding(current, persisted *config) {
	if current == nil || persisted == nil || normalizeWindowsUserSID(current.WindowsSessionUserSID) != "" {
		return
	}
	if sid := normalizeWindowsUserSID(persisted.WindowsSessionUserSID); sid != "" {
		current.WindowsSessionUserSID = sid
		current.WindowsSessionUserName = strings.TrimSpace(persisted.WindowsSessionUserName)
	}
}

func windowsSessionDisplayName(cfg *config) string {
	if cfg != nil && strings.TrimSpace(cfg.WindowsSessionUserName) != "" {
		return strings.TrimSpace(cfg.WindowsSessionUserName)
	}
	if cfg != nil && normalizeWindowsUserSID(cfg.WindowsSessionUserSID) != "" {
		return normalizeWindowsUserSID(cfg.WindowsSessionUserSID)
	}
	return "целевому пользователю"
}

func normalizeWindowsUserSID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "S-1-") {
		return ""
	}
	// Service identities can launch a silent installer, but they do not own an
	// interactive desktop. Leaving such installs unbound lets the service safely
	// adopt the first and only real user later instead of becoming permanently
	// pinned to LocalSystem/LocalService/NetworkService.
	switch value {
	case "S-1-5-18", "S-1-5-19", "S-1-5-20":
		return ""
	}
	return value
}

// selectWindowsSession enforces the privacy boundary for a machine-wide Agent:
// one Remote ID may publish exactly one Windows user's interactive session.
// There is deliberately no fallback to a different SID. On a legacy endpoint
// we auto-bind only when Windows exposes exactly one unique interactive user;
// a multi-user VDI remains unavailable until an explicit reinstall/bind.
func selectWindowsSession(candidates []windowsSessionCandidate, configuredSID string) windowsSessionSelection {
	configuredSID = normalizeWindowsUserSID(configuredSID)
	eligible := make([]windowsSessionCandidate, 0, len(candidates))
	uniqueSIDs := make(map[string]bool)
	for _, candidate := range candidates {
		candidate.UserSID = normalizeWindowsUserSID(candidate.UserSID)
		candidate.UserName = strings.TrimSpace(candidate.UserName)
		if candidate.ID == 0 || candidate.UserSID == "" || (candidate.State != windowsSessionStateActive && candidate.State != windowsSessionStateConnected && candidate.State != windowsSessionStateDisconnected) {
			continue
		}
		eligible = append(eligible, candidate)
		uniqueSIDs[candidate.UserSID] = true
	}

	targetSID := configuredSID
	autoBound := false
	if targetSID == "" {
		if len(uniqueSIDs) != 1 {
			return windowsSessionSelection{Ambiguous: len(uniqueSIDs) > 1, TargetAbsent: len(uniqueSIDs) == 0}
		}
		for sid := range uniqueSIDs {
			targetSID = sid
		}
		autoBound = true
	}

	matches := make([]windowsSessionCandidate, 0, len(eligible))
	for _, candidate := range eligible {
		if candidate.UserSID == targetSID {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return windowsSessionSelection{UserSID: targetSID, TargetAbsent: true}
	}

	// Prefer an actively attached session, then a connected one, then the user's
	// disconnected-but-still-logged-on VDI desktop. If the same account has more
	// than one session at the same state, choose the newest ID deterministically.
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].State != matches[j].State {
			return windowsSessionStateRank(matches[i].State) < windowsSessionStateRank(matches[j].State)
		}
		return matches[i].ID > matches[j].ID
	})
	selected := matches[0]
	return windowsSessionSelection{
		SessionID: selected.ID,
		UserSID:   targetSID,
		UserName:  selected.UserName,
		AutoBound: autoBound,
	}
}

func windowsSessionStateRank(state string) int {
	switch state {
	case windowsSessionStateActive:
		return 0
	case windowsSessionStateConnected:
		return 1
	case windowsSessionStateDisconnected:
		return 2
	default:
		return 3
	}
}
