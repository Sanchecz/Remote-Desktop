# RemoteIt

RemoteIt is a private remote administration platform for Windows 10/11/Server, Ubuntu/Linux and macOS, with a responsive web panel, desktop console, PWA and a signed Android APK.

## Live endpoints

- Web panel: `https://supportgenesis.ru`
- Android: `https://supportgenesis.ru/downloads/RemoteIt.apk`
- Windows installer: `https://supportgenesis.ru/downloads/RemoteIt-Agent-Setup.exe`
- Windows console: `https://supportgenesis.ru/downloads/RemoteIt-Console.exe`
- Ubuntu/macOS installer: `https://supportgenesis.ru/downloads/install-remoteit.sh`
- Checksums: `https://supportgenesis.ru/downloads/SHA256SUMS.txt`
- Android signer: `https://supportgenesis.ru/downloads/APK-SIGNER.txt`

## Implemented

- Automatic device enrollment with one-time/limited-use tokens and Remote ID.
- Public installation-code flow on the login page: an administrator can send either the code or a `#install=` link, and the recipient receives a token-bound Windows, macOS or Linux installer without access to the management panel.
- Windows service install to Program Files with tray mini-panel.
- Transparent Windows current-user fallback when UAC is unavailable; the web panel shows the real execution privilege level.
- Linux systemd and macOS LaunchDaemon installation.
- Safe in-place agent upgrades: service restart on Linux/macOS and atomic executable replacement on Windows, including current-user mode; orphaned credentials from an old panel-only deletion are detected and re-enrolled with the supplied token.
- Automatic agent updates from version 0.7.0 onward: the heartbeat advertises a same-domain HTTPS artifact, and the Agent verifies its exact size and SHA-256 before replacing the installed binary. Older Agents need one manual upgrade to 0.7.0 first.
- Inventory: public/local IPs, OS, architecture, current user, CPU, RAM, disk and online state.
- Persistent queued terminal jobs, timeouts, cancellation, bounded output, retries and audit events.
- Ready-to-use system, network, process, service and disk diagnostics.
- Device rename/group changes, device revocation, token revocation, user disabling and password reset.
- Owner/admin/technician/viewer RBAC, CSRF protection, Argon2id passwords and forced temporary-password replacement.
- Owner-only per-device password protection: the main Owner account always retains access, while every admin or technician must unlock the protected device in their own panel session before remote access, terminal, files, scenarios, rename or removal. Device passwords are stored only as Argon2id hashes and never reach the Agent.
- Account settings with password rotation, active-session inventory and remote session revocation.
- Three UI themes: dark, light and white/blue.
- Standalone Windows `RemoteIt-Console.exe` opens the management interface in its own application window and isolated login profile.
- Active agent-session view and executable diagnostic-scenario library.
- Dedicated remote-access workspace with an online-device rail, silent audited desktop preview, full-screen mouse/keyboard control, whole-screen fit/zoom modes and Auto/15/30/60 FPS selection. The remote computer is notified only when the operator starts actual input control.
- Context-aware AI administrator: real device telemetry, an OS-specific allowlisted diagnostic plan, audited command execution, analysis of returned output, and explicit confirmation before every state-changing command. It works in local diagnostic mode by default and can optionally use the OpenAI Responses API through a server-only `OPENAI_API_KEY`.
- The panel generates a token-bound Windows setup: one RemoteIt window asks for the computer name, UAC is approved once, and no PowerShell or console window is opened.
- Release-signed Android WebView app locked to the RemoteIt production domain, with the system file chooser for uploads and authenticated Android Download Manager integration for large downloads.
- The remote screen uses unified pointer input, so mouse control in the Console and direct tap/hold/drag control in the Android app share the same visible, audited session. Android also has an in-session software-keyboard bar with Unicode text, Enter and Backspace.
- Remote file browser with directory navigation, resumable drag-and-drop upload and streaming download up to 10 GiB per file; 8 MiB transport chunks keep large files out of process memory.
- Confirmed remote uninstall: the panel keeps a device pending until the Windows cleanup helper has removed the service, startup entries, installed application and local data.
- Daily PostgreSQL plus protected `.env`/Android signing-key backups with seven-day local retention and verified restore metadata.
- The plaintext owner bootstrap password is scrubbed from `.env` after account creation; only its Argon2id hash remains in PostgreSQL.

## Role boundaries

| Operation | Owner | Admin | Technician | Viewer |
|---|---:|---:|---:|---:|
| View devices | Yes | Yes | Yes | Yes |
| Inventory refresh | Yes | Yes | Yes | No |
| Remote access | Yes | Yes | Yes | No |
| Remote shell | Yes | Yes | No | No |
| See shell history | Yes | Yes | No | No |
| Manage devices/tokens | Yes | Yes | Limited rename/group | No |
| Manage admins | Yes | No | No | No |
| View security audit | Yes | Yes | No | No |
| Set/change/remove a device access password | Yes | No | No | No |

An administrator or technician who knows a protected device's password receives access for four hours in the current authenticated panel session only. Logging out, revoking that login session, pressing **Lock now**, or changing the device password removes the grant. Changing protection also ends non-owner remote-access sessions and cancels outstanding jobs and file transfers for that device.

## Windows installation

Download `RemoteIt-Agent-Setup.exe` or a token-bound Agent from the Tokens page and double-click it. Normal setup uses one RemoteIt window plus the standard Windows UAC prompt; it does not start PowerShell or a console.

For unattended deployment from an already elevated software-distribution process:

```powershell
RemoteIt-Agent-Setup.exe setup --quiet --token "ENROLLMENT_TOKEN" --name "Computer name"
```

Add `--user-mode` for a non-administrative per-user installation. A system-wide quiet launch that was not started elevated can still show the unavoidable Windows UAC consent dialog.

## Ubuntu/macOS installation

```sh
sh ./install-remoteit.sh --token "ENROLLMENT_TOKEN"
```

Without `--name`, the script asks for the computer name and falls back to the hostname in unattended mode. It accepts HTTPS only, supports either `curl` or `wget`, and verifies the selected binary against `SHA256SUMS.txt` before installation.

## Operations

- Containers use the fixed Compose project name `genesisit`.
- PostgreSQL is not exposed publicly.
- The public surface exposes only SSH, HTTP and HTTPS. Root login by key and password remains enabled by the owner's explicit recovery requirement; rotate disclosed credentials and restrict SSH by source IP when operationally possible.
- The app container is read-only, drops Linux capabilities and runs as an unprivileged user.
- A one-shot least-privilege initializer owns the transfer volume for the unprivileged app; the application cannot write elsewhere in the container.
- App/DB/Caddy containers have health checks or restart policies and bounded Docker logs.
- Database backup timer: `genesisit-backup.timer` at approximately 03:20 daily.
- Docker maintenance timer: `genesisit-docker-maintenance.timer` weekly, pruning only build/image data older than seven days.
- Backup directory: `/opt/genesisit/backups`, root-only, seven-day retention.
- Audit retention: 180 days; completed job retention: 90 days; expired sessions are cleared hourly.
- A 300-agent concurrent heartbeat test completes without errors; the latest measured p95 was below 150 ms on the production VPS.

## Portable releases and server migration

Every release is prepared as a portable source/build set without credentials. The live database, device identities, audit history, protected `.env`, Android signing material and staged transfer files are exported separately with `ops/remoteit-export-state`. A guarded restore script refuses a non-empty target. See [`docs/MIGRATION.md`](docs/MIGRATION.md) for the complete migration and rollback procedure.

Before a restore, stop writes and take a fresh backup. Validate a dump without changing the database:

```sh
pg_restore --list genesisit-YYYYMMDDTHHMMSSZ.dump
```

## Android signing

The release key is stored only under the protected server shared directory and is mounted into BuildKit as a secret. Public certificate SHA-256:

`d8a693ae3ae66393fa85cab9a14312106ac2c30d04d13742857a1392fbc1b2a6`

## Current boundary

This release includes inventory, accounts, audit, privileged command execution, chunked remote file browsing/download/upload (including drag-and-drop up to 10 GiB per file) and a Windows remote desktop with a silent audited preview followed by explicit mouse/keyboard control. The current low-latency latest-frame JPEG transport supports Auto/15/30/60 FPS targets and dynamically drops intermediate frames instead of queueing stale ones; achieved FPS still depends on the remote CPU, resolution and network. Secure-desktop/UAC capture remains outside the current desktop-session boundary.
