package main

import "testing"

func TestSelectWindowsSessionNeverFallsBackToAnotherUser(t *testing.T) {
	selection := selectWindowsSession([]windowsSessionCandidate{
		{ID: 4, UserSID: "S-1-5-21-100", UserName: `VDI\a.birukov`, State: windowsSessionStateConnected},
		{ID: 9, UserSID: "S-1-5-21-200", UserName: `VDI\other`, State: windowsSessionStateActive},
	}, "S-1-5-21-100")
	if selection.SessionID != 4 || selection.UserSID != "S-1-5-21-100" || selection.TargetAbsent || selection.Ambiguous {
		t.Fatalf("selected %+v; want only a.birukov session 4", selection)
	}
}

func TestSelectWindowsSessionRefusesAmbiguousLegacyVDI(t *testing.T) {
	selection := selectWindowsSession([]windowsSessionCandidate{
		{ID: 4, UserSID: "S-1-5-21-100", State: windowsSessionStateActive},
		{ID: 9, UserSID: "S-1-5-21-200", State: windowsSessionStateActive},
	}, "")
	if selection.SessionID != 0 || !selection.Ambiguous || selection.AutoBound {
		t.Fatalf("selected %+v; want fail-closed ambiguous result", selection)
	}
}

func TestSelectWindowsSessionAutoBindsSingleUser(t *testing.T) {
	selection := selectWindowsSession([]windowsSessionCandidate{
		{ID: 3, UserSID: "s-1-5-21-100", UserName: `VDI\a.birukov`, State: windowsSessionStateConnected},
	}, "")
	if selection.SessionID != 3 || !selection.AutoBound || selection.UserSID != "S-1-5-21-100" {
		t.Fatalf("selected %+v; want safe single-user auto-bind", selection)
	}
}

func TestSelectWindowsSessionPrefersActiveAndNewestForSameSID(t *testing.T) {
	selection := selectWindowsSession([]windowsSessionCandidate{
		{ID: 12, UserSID: "S-1-5-21-100", State: windowsSessionStateConnected},
		{ID: 7, UserSID: "S-1-5-21-100", State: windowsSessionStateActive},
		{ID: 15, UserSID: "S-1-5-21-100", State: windowsSessionStateActive},
	}, "S-1-5-21-100")
	if selection.SessionID != 15 {
		t.Fatalf("selected session %d; want newest active session 15", selection.SessionID)
	}
}

func TestSelectWindowsSessionReportsMissingBoundUser(t *testing.T) {
	selection := selectWindowsSession([]windowsSessionCandidate{
		{ID: 9, UserSID: "S-1-5-21-200", State: windowsSessionStateActive},
	}, "S-1-5-21-100")
	if selection.SessionID != 0 || !selection.TargetAbsent || selection.Ambiguous {
		t.Fatalf("selected %+v; want missing target without fallback", selection)
	}
}

func TestSelectWindowsSessionKeepsBoundDisconnectedVDIUser(t *testing.T) {
	selection := selectWindowsSession([]windowsSessionCandidate{
		{ID: 4, UserSID: "S-1-5-21-100", UserName: `VDI\a.birukov`, State: windowsSessionStateDisconnected},
		{ID: 9, UserSID: "S-1-5-21-200", UserName: `VDI\other`, State: windowsSessionStateActive},
	}, "S-1-5-21-100")
	if selection.SessionID != 4 || selection.UserSID != "S-1-5-21-100" {
		t.Fatalf("selected %+v; want disconnected a.birukov session 4 without fallback", selection)
	}
}

func TestNormalizeWindowsUserSIDRejectsServiceIdentities(t *testing.T) {
	for _, sid := range []string{"S-1-5-18", "s-1-5-19", " S-1-5-20 "} {
		if got := normalizeWindowsUserSID(sid); got != "" {
			t.Fatalf("normalizeWindowsUserSID(%q) = %q; want empty service identity", sid, got)
		}
	}
}

func TestMergeWindowsSessionBindingProtectsPersistedPrivacyBoundary(t *testing.T) {
	current := &config{DeviceID: "device"}
	persisted := &config{WindowsSessionUserSID: "s-1-5-21-100", WindowsSessionUserName: `VDI\a.birukov`}
	mergeWindowsSessionBinding(current, persisted)
	if current.WindowsSessionUserSID != "S-1-5-21-100" || current.WindowsSessionUserName != `VDI\a.birukov` {
		t.Fatalf("merged %+v; want persisted VDI owner", current)
	}
}

func TestMergeWindowsSessionBindingAllowsExplicitRebind(t *testing.T) {
	current := &config{WindowsSessionUserSID: "S-1-5-21-200", WindowsSessionUserName: `VDI\new`}
	persisted := &config{WindowsSessionUserSID: "S-1-5-21-100", WindowsSessionUserName: `VDI\old`}
	mergeWindowsSessionBinding(current, persisted)
	if current.WindowsSessionUserSID != "S-1-5-21-200" || current.WindowsSessionUserName != `VDI\new` {
		t.Fatalf("merged %+v; explicit rebind must win", current)
	}
}
