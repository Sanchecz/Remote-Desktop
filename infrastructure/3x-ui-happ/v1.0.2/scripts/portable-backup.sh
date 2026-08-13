#!/usr/bin/env bash
set -euo pipefail

umask 077

role="${1:-}"
case "$role" in
  main)
    health_name="openai-chain-main-healthcheck"
    ;;
  node2)
    health_name="openai-chain-node2-healthcheck"
    ;;
  *)
    echo "usage: $0 main|node2" >&2
    exit 64
    ;;
esac

task_stamp="$(date -u +%Y%m%dT%H%M%SZ)"
task_root="/root/codex-backups/portable-current-${role}-${task_stamp}"
task_stage="${task_root}/snapshot"
task_archive="${task_root}.tar.gz"

install -d -m 700 "$task_stage"

cp -a --parents \
  /etc/x-ui \
  /usr/local/x-ui \
  /etc/systemd/system/x-ui.service \
  /root/cert \
  /root/.acme.sh \
  /root/openai-chain \
  "/usr/local/sbin/${health_name}" \
  "/etc/systemd/system/${health_name}.service" \
  "/etc/systemd/system/${health_name}.timer" \
  "$task_stage"

timer_link="/etc/systemd/system/timers.target.wants/${health_name}.timer"
if [[ -e "$timer_link" || -L "$timer_link" ]]; then
  cp -a --parents "$timer_link" "$task_stage"
fi

rm -f \
  "$task_stage/etc/x-ui/x-ui.db" \
  "$task_stage/etc/x-ui/x-ui.db-wal" \
  "$task_stage/etc/x-ui/x-ui.db-shm"

python3 - "$task_stage/etc/x-ui/x-ui.db" "$task_root/DB-INTEGRITY.txt" <<'PY'
import sqlite3
import sys

source = sqlite3.connect("file:/etc/x-ui/x-ui.db?mode=ro", uri=True)
target = sqlite3.connect(sys.argv[1])
source.backup(target)
result = target.execute("PRAGMA integrity_check").fetchone()[0]
target.close()
source.close()
with open(sys.argv[2], "w", encoding="utf-8") as report:
    report.write(f"sqlite_integrity_check={result}\n")
if result != "ok":
    raise SystemExit("SQLite integrity check failed")
PY

chmod --reference=/etc/x-ui/x-ui.db "$task_stage/etc/x-ui/x-ui.db"

{
  printf 'snapshot_utc='; date -u +%Y-%m-%dT%H:%M:%SZ
  printf 'role=%s\n' "$role"
  printf 'hostname='; hostname
  printf 'xui_pid='; pgrep -xo x-ui || true
  printf 'xray_pid='; ps -eo pid=,comm= | awk '$2 ~ /^xray/ {print $1; exit}' || true
  printf 'xui_active='; systemctl is-active x-ui || true
  printf 'xui_enabled='; systemctl is-enabled x-ui || true
  printf 'health_timer_active='; systemctl is-active "${health_name}.timer" || true
  printf 'health_timer_enabled='; systemctl is-enabled "${health_name}.timer" || true
  printf 'kernel='; uname -r
} > "$task_root/STATE.txt"

cp /etc/os-release "$task_root/OS-RELEASE.txt"
dpkg-query -W > "$task_root/PACKAGES.tsv"
crontab -l > "$task_root/ROOT-CRONTAB.txt" 2>&1 || true
ip -brief address > "$task_root/IP-ADDRESS.txt"
ip route show table all > "$task_root/ROUTES.txt"
ip rule show > "$task_root/IP-RULES.txt"
ss -lntup > "$task_root/LISTENERS.txt"
iptables-save > "$task_root/IPTABLES-v4.txt" 2>&1 || true
ip6tables-save > "$task_root/IPTABLES-v6.txt" 2>&1 || true
nft list ruleset > "$task_root/NFTABLES.txt" 2>&1 || true
systemctl --no-pager --full status x-ui "${health_name}.timer" \
  > "$task_root/SERVICES.txt" 2>&1 || true

{
  /usr/local/x-ui/x-ui -v || true
  /usr/local/x-ui/bin/xray-linux-amd64 version || true
} > "$task_root/VERSIONS.txt" 2>&1

(
  cd "$task_root"
  find snapshot -type f -print0 \
    | LC_ALL=C sort -z \
    | xargs -0 sha256sum \
    > SNAPSHOT-SHA256SUMS.txt
)

tar -C "$(dirname "$task_root")" \
  -czf "$task_archive" \
  "$(basename "$task_root")"
chmod 600 "$task_archive"

printf 'PORTABLE_ARCHIVE=%s\n' "$task_archive"
printf 'PORTABLE_SHA256='
sha256sum "$task_archive" | awk '{print $1}'
printf 'PORTABLE_SIZE='
stat -c %s "$task_archive"
