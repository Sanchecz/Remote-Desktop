//go:build !windows

package main

import "errors"

type setupInstallPayload struct {
	Token      string
	Name       string
	ServerURL  string
	UserMode   bool
	ResultFile string
}

func loadSetupInstallPayload(string) (setupInstallPayload, error) {
	return setupInstallPayload{}, errors.New("GUI-параметры установки доступны только в Windows")
}

func recordInstallResult(string, error) {}

func appendInstallDiagnostic(error) {}
