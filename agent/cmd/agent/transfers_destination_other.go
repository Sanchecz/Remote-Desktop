//go:build !windows

package main

import "errors"

func remoteUserDesktopDirectory(_ *config) (string, error) {
	return "", errors.New("загрузка на рабочий стол поддерживается Windows Agent")
}

func remoteUserKnownDirectory(_ *config, _ string) (string, error) {
	return "", errors.New("загрузка в пользовательские папки поддерживается Windows Agent")
}
