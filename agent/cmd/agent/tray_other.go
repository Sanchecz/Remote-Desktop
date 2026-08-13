//go:build !windows

package main

import "errors"

func trayCommand() error {
	return errors.New("значок в трее пока доступен только в Windows-версии RemoteIt Agent")
}
