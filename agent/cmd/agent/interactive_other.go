//go:build !windows

package main

import (
	"context"
	"errors"
)

func runInteractiveCompanionBroker(context.Context) {}

func desktopWorkerCommand() error {
	return errors.New("desktop-worker is supported only on Windows")
}
