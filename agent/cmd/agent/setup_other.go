//go:build !windows

package main

import "errors"

func setupCommand() error {
	return errors.New("графическая установка доступна только в Windows-версии RemoteIt Agent")
}
