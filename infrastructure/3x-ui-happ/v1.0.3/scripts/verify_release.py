#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / "SHA256SUMS.txt"
sys.path.insert(0, str(Path(__file__).resolve().parent))
from build_happ_link import load_and_validate, sha256  # noqa: E402


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> int:
    if not MANIFEST.is_file():
        raise SystemExit("Файл SHA256SUMS.txt отсутствует")

    checked: set[str] = set()
    for number, raw in enumerate(MANIFEST.read_text(encoding="utf-8").splitlines(), 1):
        if not raw.strip():
            continue
        try:
            expected, relative = raw.split("  ", 1)
        except ValueError as exc:
            raise SystemExit(f"Некорректная строка manifest #{number}") from exc
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"Файл отсутствует: {relative}")
        if file_sha256(path).lower() != expected.lower():
            raise SystemExit(f"SHA-256 не совпадает: {relative}")
        checked.add(relative.replace("\\", "/"))

    forbidden_suffixes = {
        ".db",
        ".sqlite",
        ".sqlite3",
        ".key",
        ".pem",
        ".gz",
        ".7z",
        ".zip",
        ".tar",
    }
    private_key_markers = tuple(
        "BEGIN " + prefix + "PRIVATE KEY"
        for prefix in ("", "RSA ", "EC ", "OPENSSH ")
    )
    actual_files: set[str] = set()
    for path in ROOT.rglob("*"):
        if not path.is_file() or path == MANIFEST or "__pycache__" in path.parts:
            continue
        relative = path.relative_to(ROOT).as_posix()
        actual_files.add(relative)
        if path.suffix.lower() in forbidden_suffixes:
            raise SystemExit(f"Запрещённый секретный/архивный файл в kit: {relative}")
        if path.suffix.lower() in {".md", ".py", ".sh", ".json", ".txt"}:
            text = path.read_text(encoding="utf-8")
            if "\ufffd" in text or any(marker in text for marker in private_key_markers):
                raise SystemExit(f"Повреждённый UTF-8 или приватный ключ: {relative}")

    missing_from_manifest = sorted(actual_files - checked)
    stale_manifest = sorted(checked - actual_files)
    if missing_from_manifest or stale_manifest:
        raise SystemExit(
            f"Manifest не покрывает комплект: missing={missing_from_manifest}, stale={stale_manifest}"
        )

    routing = ROOT / "routing" / "RoscomVPN.routing.json"
    profile, compact, link = load_and_validate(routing)
    print(f"RELEASE_HASHES_OK={len(checked)}")
    print(f"ROUTING_KEYS_OK={len(profile)}")
    print(f"ROUTING_COMPACT_SHA256={sha256(compact)}")
    print(f"ROUTING_DEEPLINK_SHA256={sha256(link.encode('utf-8'))}")
    print("RELEASE_VALIDATION_OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
