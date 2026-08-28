const integrationTokenPattern = /^rmt_mcp_[A-Za-z0-9_-]{32,}$/;

function normalizedRemoteItOrigin(value: string) {
	const url = new URL(value);
	const localDevelopment = url.protocol === "http:" && (url.hostname === "localhost" || url.hostname === "127.0.0.1");
	if (url.protocol !== "https:" && !localDevelopment) throw new Error("RemoteIt MCP requires an HTTPS server");
	if (url.username || url.password || url.search || url.hash) throw new Error("Invalid RemoteIt server URL");
	return url.origin;
}

export function buildWindowsMCPInstaller(baseURL: string, token: string) {
	const origin = normalizedRemoteItOrigin(baseURL);
	if (!integrationTokenPattern.test(token)) throw new Error("Invalid RemoteIt integration token");
	return `@echo off
setlocal EnableExtensions DisableDelayedExpansion
title RemoteIt AI Administrator Setup
set "REMOTEIT_URL=${origin}"
set "REMOTEIT_TOKEN=${token}"
set "REMOTEIT_DIR=%LOCALAPPDATA%\\RemoteIt\\MCP"
set "REMOTEIT_EXE=%REMOTEIT_DIR%\\RemoteIt-MCP.exe"
set "REMOTEIT_SHA_FILE=%TEMP%\\RemoteIt-MCP-SHA256SUMS.txt"

echo [1/4] Checking Codex...
set "CODEX_CMD="
if defined CODEX_CLI_PATH if exist "%CODEX_CLI_PATH%" set "CODEX_CMD=%CODEX_CLI_PATH%"
if not defined CODEX_CMD for /f "usebackq delims=" %%C in (\`powershell.exe -NoLogo -NoProfile -NonInteractive -Command "$candidate = Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'OpenAI\\Codex\\bin') -Filter codex.exe -File -Recurse -ErrorAction SilentlyContinue ^| Sort-Object LastWriteTimeUtc -Descending ^| Select-Object -First 1 -ExpandProperty FullName; if ($candidate) { $candidate }"\`) do set "CODEX_CMD=%%C"
if not defined CODEX_CMD for /f "delims=" %%C in ('where codex.exe 2^>nul') do if not defined CODEX_CMD set "CODEX_CMD=%%C"
if not defined CODEX_CMD goto codex_missing
"%CODEX_CMD%" --version >nul 2>&1 || goto codex_missing

echo [2/4] Downloading verified RemoteIt MCP...
if not exist "%REMOTEIT_DIR%" mkdir "%REMOTEIT_DIR%" || goto failed
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; $base=$env:REMOTEIT_URL.TrimEnd('/'); Invoke-WebRequest -UseBasicParsing -Uri ($base + '/downloads/RemoteIt-MCP.exe') -OutFile $env:REMOTEIT_EXE; Invoke-WebRequest -UseBasicParsing -Uri ($base + '/downloads/SHA256SUMS.txt') -OutFile $env:REMOTEIT_SHA_FILE" || goto failed

echo [3/4] Verifying SHA-256...
set "EXPECTED_SHA="
set "ACTUAL_SHA="
for /f "tokens=1" %%H in ('findstr /I "RemoteIt-MCP.exe" "%REMOTEIT_SHA_FILE%"') do set "EXPECTED_SHA=%%H"
for /f %%H in ('powershell.exe -NoLogo -NoProfile -NonInteractive -Command "(Get-FileHash -Algorithm SHA256 -LiteralPath $env:REMOTEIT_EXE).Hash.ToLowerInvariant()"') do set "ACTUAL_SHA=%%H"
if not defined EXPECTED_SHA goto hash_failed
if not defined ACTUAL_SHA goto hash_failed
if /I not "%EXPECTED_SHA%"=="%ACTUAL_SHA%" goto hash_failed

echo [4/4] Connecting RemoteIt to Codex...
"%CODEX_CMD%" mcp remove remoteit >nul 2>&1
"%CODEX_CMD%" mcp add remoteit --env REMOTEIT_URL="%REMOTEIT_URL%" --env REMOTEIT_INTEGRATION_TOKEN="%REMOTEIT_TOKEN%" -- "%REMOTEIT_EXE%" || goto failed

del /q "%REMOTEIT_SHA_FILE%" >nul 2>&1
set "REMOTEIT_TOKEN="
echo.
echo RemoteIt AI Administrator is connected. Restart Codex if RemoteIt tools do not appear immediately.
echo This personal setup file will now be deleted because it contained an access token.
timeout /t 4 /nobreak >nul
del /q "%~f0" & exit /b 0

:codex_missing
echo.
echo [ERROR] Codex CLI was not found. Install or open Codex on this computer and run this file again.
goto keep_file

:hash_failed
del /q "%REMOTEIT_EXE%" "%REMOTEIT_SHA_FILE%" >nul 2>&1
echo.
echo [ERROR] RemoteIt MCP integrity verification failed. Nothing was connected.
goto keep_file

:failed
echo.
echo [ERROR] RemoteIt AI Administrator setup did not finish.

:keep_file
echo Revoke this integration in RemoteIt if the setup file is lost or no longer needed.
pause
exit /b 1
`;
}

export function buildCodexOperatorInstruction(baseURL: string) {
	const origin = normalizedRemoteItOrigin(baseURL);
	return `REMOTEIT AI-АДМИНИСТРАТОР: ИНСТРУКЦИЯ ДЛЯ CODEX

Сервер RemoteIt: ${origin}

Используй подключённые инструменты RemoteIt MCP. Не запрашивай пароль от панели RemoteIt и никогда не помещай пароли, токены или приватные ключи в команды, скрипты, сообщения чата и репозитории.

ПОРЯДОК РАБОТЫ С КАЖДЫМ ЗАПРОСОМ
1. Найди целевой компьютер строго по его Remote ID и покажи название, операционную систему, состояние сети и версию Agent.
2. До любых изменений прочитай текущее состояние устройства.
3. Предпочитай типизированное действие RemoteIt. Используй script.execute только тогда, когда задачу нельзя выразить безопасным типизированным действием.
4. Перед изменением покажи точную цель, параметры, команды, риск и способ отката.
5. Создай задание RemoteIt и дождись его проверки владельцем или администратором в панели. Критические действия и script.execute подтверждает владелец.
6. После подтверждения дождись завершения, проверь требуемый результат и сообщи ID задания, код завершения и значимый вывод.
7. Никогда не выполняй действие на другом устройстве, если указанный Remote ID отсутствует, неоднозначен или находится не в сети.

ГОТОВЫЙ ЗАПРОС ДЛЯ ДРУГОГО CODEX
Используй RemoteIt MCP.
Remote ID: <REMOTE_ID>.
Задача: <ЧТО НУЖНО СДЕЛАТЬ>.
Сначала проверь устройство и подготовь безопасный план. Не изменяй устройство, пока действие не подтверждено в панели RemoteIt. После выполнения проверь результат и сообщи ID задания и вывод.

Remote ID — это идентификатор, а не пароль. Доступ разрешается только через персональную MCP-интеграцию владельца или администратора RemoteIt.
`;
}
