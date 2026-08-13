#!/usr/bin/env python3
from __future__ import annotations

import argparse
import base64
import json
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Собрать happ://routing/add ссылку из routing JSON."
    )
    parser.add_argument("json_file", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    profile = json.loads(args.json_file.read_text(encoding="utf-8"))
    compact = json.dumps(
        profile, ensure_ascii=False, separators=(",", ":")
    ).encode("utf-8")
    link = "happ://routing/add/" + base64.b64encode(compact).decode("ascii")
    if args.output:
        args.output.write_text(link + "\n", encoding="utf-8")
    else:
        print(link)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
