#!/usr/bin/env python3
"""Check merged coverage against the project gates.

Usage: coverage-check.py <profile> <total_min> <core_min>
The core floor applies to packages with unit-level suites; external-service
adapters (MQTT broker, Qdrant, Postgres) are covered where test doubles exist and
are intentionally not part of the per-package floor.
"""
import sys
from collections import defaultdict

CORE = [
    "internal/platform",
    "internal/security",
    "internal/config",
    "internal/voice",
    "internal/adapters/vector",
    "internal/memory",
    "internal/brain",
    "internal/devices",
    "internal/observability",
]


def main() -> int:
    profile, total_min, core_min = sys.argv[1], float(sys.argv[2]), float(sys.argv[3])
    totals = defaultdict(lambda: [0, 0])
    with open(profile, encoding="utf-8") as fh:
        for line in fh:
            if line.startswith("mode:"):
                continue
            loc, num, count = line.strip().rsplit(" ", 2)
            pkg = "/".join(loc.split(":")[0].split("/")[:-1])
            totals[pkg][0] += int(num)
            if int(count) > 0:
                totals[pkg][1] += int(num)
    total = sum(t for t, _ in totals.values())
    covered = sum(c for _, c in totals.values())
    pct = 100 * covered / total if total else 0
    print(f"total coverage: {pct:.2f}% ({covered}/{total} statements, minimum {total_min:.0f}%)")
    failed = pct + 1e-9 < total_min
    for name in CORE:
        key = f"github.com/ArsinShaabani/AIOS/{name}"
        if key not in totals:
            print(f"FAIL missing package {name}")
            failed = True
            continue
        t, c = totals[key]
        p = 100 * c / t if t else 0
        status = "ok  " if p + 1e-9 >= core_min else "FAIL"
        print(f"{status} {name:32s} {p:6.2f}% ({c}/{t})")
        if p + 1e-9 < core_min:
            failed = True
    if failed:
        print(f"coverage gates failed (total >= {total_min:.0f}%, core >= {core_min:.0f}%)", file=sys.stderr)
        return 1
    print("coverage gates passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
