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
EXPECTED_COMPACT_SHA256 = "239cf1eebc297157ff13fa06b665045562d9629c3146469bfec2b0c9fac962c4"
EXPECTED_DEEPLINK_SHA256 = "09bd9a430450e4e7f3c439876a9a05c82000b90d4cb2a47550f8526a55909283"


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
        "DirectIp": 12,
        "ProxySites": 44,
        "ProxyIp": 12,
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
    if geo_refs != ["geoip:ru"]:
        raise ValueError("Routing не должен зависеть от geosite:/geoip: секций")

    required_proxy_sites = {
        "domain:t.me",
        "domain:telegram.org",
        "domain:telegram-cdn.org",
        "domain:telegramdownload.com",
    }
    required_proxy_ip = {
        "91.108.4.0/22",
        "91.108.20.0/22",
        "149.154.160.0/20",
        "2001:67c:4e8::/48",
        "2001:b28:f23d::/48",
    }
    if not required_proxy_sites.issubset(profile["ProxySites"]):
        raise ValueError("Routing не покрывает основные Telegram-домены")
    if not required_proxy_ip.issubset(profile["ProxyIp"]):
        raise ValueError("Routing не покрывает IPv4/IPv6-сети Telegram")

    compact = json.dumps(
        profile, ensure_ascii=False, separators=(",", ":")
    ).encode("utf-8")
    link = "happ://routing/add/" + base64.b64encode(compact).decode("ascii")

    if len(compact) != 23564 or sha256(compact) != EXPECTED_COMPACT_SHA256:
        raise ValueError("Компактный routing не совпадает с выпуском v1.0.3")
    if len(link) != 31439 or sha256(link.encode("utf-8")) != EXPECTED_DEEPLINK_SHA256:
        raise ValueError("Happ deeplink не совпадает с выпуском v1.0.3")
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
