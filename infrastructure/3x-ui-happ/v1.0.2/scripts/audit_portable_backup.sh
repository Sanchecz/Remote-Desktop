#!/usr/bin/env bash
set -euo pipefail

archive="${1:-}"
expected_role="${2:-}"
if [[ -z "$archive" || ! -f "$archive" || ! "$expected_role" =~ ^(main|node2)$ ]]; then
  echo "Использование: $0 /path/to/portable.tar.gz main|node2" >&2
  exit 64
fi

umask 077
stage="$(mktemp -d /root/xui-backup-audit.XXXXXX)"
case "$stage" in
  /root/xui-backup-audit.*) ;;
  *) echo "Небезопасный временный путь: $stage" >&2; exit 1 ;;
esac
trap 'rm -rf -- "$stage"' EXIT

python3 - "$archive" <<'PY'
import sys
import tarfile
from pathlib import PurePosixPath

with tarfile.open(sys.argv[1], "r:gz") as bundle:
    members = bundle.getmembers()
    if not members:
        raise SystemExit("Архив пуст")
    for member in members:
        path = PurePosixPath(member.name)
        if path.is_absolute() or ".." in path.parts:
            raise SystemExit(f"Небезопасный путь в архиве: {member.name}")
print(f"TAR_PATHS_OK={len(members)}")
PY

tar -xzf "$archive" -C "$stage"
mapfile -t roots < <(find "$stage" -mindepth 1 -maxdepth 1 -type d -print)
if [[ "${#roots[@]}" -ne 1 ]]; then
  echo "Ожидался ровно один верхний каталог" >&2
  exit 1
fi
root="${roots[0]}"

grep -qx "role=${expected_role}" "$root/STATE.txt"
(
  cd "$root"
  sha256sum -c --quiet SNAPSHOT-SHA256SUMS.txt
)

db="$root/snapshot/etc/x-ui/x-ui.db"
python3 - "$db" <<'PY'
import sqlite3
import sys

db = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
result = db.execute("PRAGMA integrity_check").fetchone()[0]
print(f"SQLITE_INTEGRITY={result}")
if result != "ok":
    raise SystemExit(1)
PY

test -f "$root/snapshot/usr/local/x-ui/x-ui"
test -f "$root/snapshot/usr/local/x-ui/bin/xray-linux-amd64"
test -f "$root/snapshot/etc/systemd/system/x-ui.service"
echo "AUDIT_OK role=${expected_role}"
