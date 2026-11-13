#!/usr/bin/env python3
"""Merge Go coverprofiles by summing counts for identical blocks.

`go test -coverprofile` with several packages (or repeated test binaries) emits
the same block more than once. Naive tools then double-count statements. This
merger produces one profile with unique blocks and summed hit counts, which is
what `go tool cover -func` expects.
"""
import sys


def main(paths: list[str], out: str) -> None:
    blocks: dict[tuple[str, str], int] = {}
    for path in paths:
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                line = line.strip()
                if not line or line.startswith("mode:"):
                    continue
                loc, num, count = line.rsplit(" ", 2)
                key = (loc, num)
                blocks[key] = blocks.get(key, 0) + int(count)
    if not blocks:
        raise SystemExit("no coverage blocks found")
    with open(out, "w", encoding="utf-8") as fh:
        fh.write("mode: atomic\n")
        for (loc, num), count in sorted(blocks.items()):
            fh.write(f"{loc} {num} {count}\n")
    print(f"merged {len(paths)} profiles -> {len(blocks)} unique blocks")


if __name__ == "__main__":
    if len(sys.argv) < 3:
        raise SystemExit("usage: merge-coverage.py <out> <profile>...")
    main(sys.argv[2:], sys.argv[1])
