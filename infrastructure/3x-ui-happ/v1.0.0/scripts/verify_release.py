#!/usr/bin/env python3
from __future__ import annotations

import hashlib
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / "SHA256SUMS.txt"


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> int:
    if not MANIFEST.is_file():
        raise SystemExit("Файл SHA256SUMS.txt отсутствует")
    checked = 0
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
        actual = sha256(path)
        if actual.lower() != expected.lower():
            raise SystemExit(f"SHA-256 не совпадает: {relative}")
        checked += 1
    print(f"RELEASE_HASHES_OK={checked}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
