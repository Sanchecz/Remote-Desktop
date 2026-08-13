#!/usr/bin/env python3
from __future__ import annotations

import argparse
import base64
import hashlib
import json
from pathlib import Path
from typing import Any


EXPECTED_KEYS = (
    "Name",
    "GlobalProxy",
    "UseChunkFiles",
    "RemoteDNSType",
    "RemoteDNSDomain",
    "RemoteDNSIP",
    "DomesticDNSType",
    "DomesticDNSDomain",
    "DomesticDNSIP",
    "Geoipurl",
    "Geositeurl",
    "LastUpdated",
    "DnsHosts",
    "RouteOrder",
    "DirectSites",
    "DirectIp",
    "ProxySites",
    "ProxyIp",
    "BlockSites",
    "BlockIp",
    "DomainStrategy",
    "FakeDNS",
)
EXPECTED_COMPACT_SHA256 = "9c1c83483fccdce7f7ddb127863b36699d766d407422d021b2e8cbd3c1fbfb9a"
EXPECTED_DEEPLINK_SHA256 = "46f76e0a81e24d28e8f7bb492baeaab4779cda71adf8436b0d9b95f63fb4976b"


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def reject_casefold_duplicates(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    folded: set[str] = set()
    result: dict[str, Any] = {}
    for key, value in pairs:
        normalized = key.casefold()
        if normalized in folded:
            raise ValueError(f"Дублирующийся JSON-ключ без учёта регистра: {key}")
        folded.add(normalized)
        result[key] = value
    return result


def load_and_validate(path: Path) -> tuple[dict[str, Any], bytes, str]:
    profile = json.loads(
        path.read_text(encoding="utf-8"),
        object_pairs_hook=reject_casefold_duplicates,
    )
    if tuple(profile) != EXPECTED_KEYS:
        raise ValueError("Набор или порядок канонических ключей routing изменён")

    for key in ("GlobalProxy", "UseChunkFiles", "FakeDNS"):
        if not isinstance(profile[key], str) or profile[key] not in {"true", "false"}:
            raise ValueError(f"{key} должен быть строкой true/false")

    expected_counts = {
        "DirectSites": 469,
        "DirectIp": 11,
        "ProxySites": 18,
        "ProxyIp": 0,
        "BlockSites": 431,
        "BlockIp": 0,
    }
    for key, expected in expected_counts.items():
        value = profile[key]
        if not isinstance(value, list) or len(value) != expected:
            raise ValueError(f"{key}: ожидалось {expected} элементов")

    if profile["Geoipurl"] != "" or profile["Geositeurl"] != "":
        raise ValueError("GEO URL должны оставаться пустыми")

    geo_refs = [
        item
        for key in expected_counts
        for item in profile[key]
        if isinstance(item, str) and item.startswith(("geosite:", "geoip:"))
    ]
    if geo_refs:
        raise ValueError("Routing не должен зависеть от geosite:/geoip: секций")

    compact = json.dumps(
        profile, ensure_ascii=False, separators=(",", ":")
    ).encode("utf-8")
    link = "happ://routing/add/" + base64.b64encode(compact).decode("ascii")

    if len(compact) != 22784 or sha256(compact) != EXPECTED_COMPACT_SHA256:
        raise ValueError("Компактный routing не совпадает со стабильным v1.0.2")
    if len(link) != 30399 or sha256(link.encode("utf-8")) != EXPECTED_DEEPLINK_SHA256:
        raise ValueError("Happ deeplink не совпадает со стабильным v1.0.2")
    return profile, compact, link


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Проверить routing JSON и собрать happ://routing/add deeplink."
    )
    parser.add_argument("json_file", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--print-hashes", action="store_true")
    args = parser.parse_args()

    profile, compact, link = load_and_validate(args.json_file)
    if args.output:
        args.output.write_text(link + "\n", encoding="utf-8")
    else:
        print(link)
    if args.print_hashes:
        print(f"KEYS={len(profile)}")
        print(f"COMPACT_BYTES={len(compact)}")
        print(f"COMPACT_SHA256={sha256(compact)}")
        print(f"DEEPLINK_BYTES={len(link)}")
        print(f"DEEPLINK_SHA256={sha256(link.encode('utf-8'))}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
