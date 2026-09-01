//go:build !windows

package main

import "context"

func executeInteractiveNetworkClient(context.Context, string, string, int, string) remoteJobResult {
	return failedAction("внутренние RDP/SSH-подключения пока поддерживаются только Windows Agent")
}

func executeWindowsPrinterList(context.Context) remoteJobResult {
	return failedAction("управление принтерами поддерживается только Windows Agent")
}

func executeWindowsPrinterSettings(context.Context) remoteJobResult {
	return failedAction("управление принтерами поддерживается только Windows Agent")
}

func executeWindowsPrinterSetDefault(context.Context, string) remoteJobResult {
	return failedAction("управление принтерами поддерживается только Windows Agent")
}

func executeWindowsScanFolderConfigure(context.Context, string, string, string) remoteJobResult {
	return failedAction("папка сканов поддерживается только Windows Agent")
}
