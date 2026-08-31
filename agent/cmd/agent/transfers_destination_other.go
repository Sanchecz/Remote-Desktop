//go:build !windows

package main

import "errors"

func remoteUserDesktopDirectory(_ *config) (string, error) {
	return "", errors.New("загрузка на рабочий стол поддерживается Windows Agent")
}
