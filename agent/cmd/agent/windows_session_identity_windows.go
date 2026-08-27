//go:build windows

package main

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// currentInstallSessionOwner runs before the setup process elevates. The SID
// therefore identifies the Windows user who intentionally launched the Agent
// installer, not LocalSystem and not whichever VDI session happens to be active
// later. Session IDs are ephemeral across reconnects, while the SID is stable.
func currentInstallSessionOwner() (string, string, error) {
	token := windows.GetCurrentProcessToken()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", "", fmt.Errorf("пользователь установщика Windows: %w", err)
	}
	if tokenUser == nil || tokenUser.User.Sid == nil {
		return "", "", fmt.Errorf("Windows не вернула SID пользователя установщика")
	}
	sid := normalizeWindowsUserSID(tokenUser.User.Sid.String())
	account, domain, _, lookupErr := tokenUser.User.Sid.LookupAccount("")
	name := strings.TrimSpace(account)
	if strings.TrimSpace(domain) != "" && name != "" {
		name = strings.TrimSpace(domain) + `\` + name
	}
	if lookupErr != nil {
		// The SID alone is authoritative. A domain controller can be temporarily
		// unavailable during installation, so a cosmetic name lookup must not
		// make an otherwise valid unattended install fail.
		name = ""
	}
	return sid, name, nil
}

func prepareWindowsSessionBinding(cfg *config) (bool, error) {
	if cfg == nil || normalizeWindowsUserSID(cfg.WindowsSessionUserSID) != "" {
		return false, nil
	}
	candidates, err := windowsSessionCandidates()
	if err != nil {
		return false, err
	}
	selection := selectWindowsSession(candidates, "")
	if selection.Ambiguous {
		return false, fmt.Errorf("обнаружено несколько пользователей VDI; повторно запустите установщик из нужного сеанса")
	}
	if !selection.AutoBound || selection.UserSID == "" {
		return false, nil
	}
	cfg.WindowsSessionUserSID = selection.UserSID
	cfg.WindowsSessionUserName = selection.UserName
	return true, nil
}
