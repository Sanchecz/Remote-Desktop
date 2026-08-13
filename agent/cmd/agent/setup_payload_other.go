//go:build !windows

package main

import "errors"

type setupInstallPayload struct {
	Token     string
	Name      string
	ServerURL string
	UserMode  bool
}

func loadSetupInstallPayload(string) (setupInstallPayload, error) {
	return setupInstallPayload{}, errors.New("GUI-параметры установки доступны только в Windows")
}
