#!/usr/bin/env python3
"""Compare complete tala-performance JSONL runs, rejecting changed outputs."""

import argparse
import collections
import csv
import json
import statistics
import sys


def read(path):
    cases = collections.defaultdict(list)
    seen = set()
    with open(path) as f:
        for line in f:
            row = json.loads(line)
            key = row["name"], row["iteration"]
            if key in seen:
                raise ValueError(f"duplicate case/iteration in {path}: {key}")
            seen.add(key)
            cases[row["name"]].append(row)
    if not cases:
        raise ValueError(f"empty run: {path}")
    iterations = {frozenset(r["iteration"] for r in rows) for rows in cases.values()}
    if len(iterations) != 1:
        raise ValueError(f"incomplete corpus repetition in {path}")
    return cases


def fingerprint(row):
    keys = ("input_sha256", "error", "graph_sha256", "diagram_sha256", "svg_sha256", "reroute_sha256")
    return tuple(row.get(k, "") for k in keys)


def median_ns(rows, fields):
    return statistics.median(sum(r.get(k, 0) for k in fields) for r in rows)


def suite_totals(cases, fields):
    totals = collections.defaultdict(int)
    for rows in cases.values():
        for row in rows:
            totals[row["iteration"]] += sum(row.get(k, 0) for k in fields)
    return [total / 1e9 for _, total in sorted(totals.items())]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("baseline")
    parser.add_argument("candidate")
    parser.add_argument("--csv", help="optional private per-case timing output")
    parser.add_argument("--expected-errors", help="external JSON mapping from case name to exact known baseline error")
    args = parser.parse_args()
    baseline, candidate = read(args.baseline), read(args.candidate)
    if baseline.keys() != candidate.keys():
        raise ValueError(f"case sets differ: missing={sorted(baseline.keys() - candidate.keys())}, extra={sorted(candidate.keys() - baseline.keys())}")
    expected_errors = {}
    if args.expected_errors:
        with open(args.expected_errors) as f:
            expected_errors = json.load(f)
        if not isinstance(expected_errors, dict) or any(not isinstance(v, str) or not v for v in expected_errors.values()):
            raise ValueError("expected errors must map case names to nonempty error strings")
        if expected_errors.keys() - baseline.keys():
            raise ValueError("expected errors refer to cases outside the compared corpus")
    changed, unstable, failures, expected_failures, unexpected_errors, rows = [], [], [], [], [], []
    for name in sorted(baseline):
        before, after = baseline[name], candidate[name]
        before_fp, after_fp = {fingerprint(r) for r in before}, {fingerprint(r) for r in after}
        if len(before_fp) != 1 or len(after_fp) != 1:
            unstable.append(name)
        if before_fp != after_fp:
            changed.append(name)
        if any(r.get("error") for r in before + after):
            failures.append(name)
        expected_error = expected_errors.get(name, "")
        if any(r.get("error", "") != expected_error for r in before + after):
            unexpected_errors.append(name)
        elif expected_error:
            expected_failures.append(name)
        b = median_ns(before, ("layout_ns", "routing_ns"))
        a = median_ns(after, ("layout_ns", "routing_ns"))
        rows.append({"name": name, "baseline_ns": b, "candidate_ns": a, "speedup": b / a if a else None})
    if args.csv:
        with open(args.csv, "w", newline="") as f:
            writer = csv.DictWriter(f, fieldnames=("name", "baseline_ns", "candidate_ns", "speedup"))
            writer.writeheader()
            writer.writerows(rows)
    metrics = {}
    for label, fields in {
        "tala": ("layout_ns", "routing_ns"),
        "layout": ("layout_ns",),
        "routing": ("routing_ns",),
        "compile": ("compile_ns",),
        "compile_and_render": ("compile_ns", "render_ns"),
        "reroute": ("reroute_ns",),
    }.items():
        b, a = suite_totals(baseline, fields), suite_totals(candidate, fields)
        bm, am = statistics.median(b), statistics.median(a)
        metrics[label] = {"baseline_seconds": b, "candidate_seconds": a, "median_speedup": bm / am if am else None}
    print(json.dumps({"cases": len(rows), "successful_cases": len(rows) - len(failures),
                      "changed_outputs": changed, "unstable_outputs": unstable,
                      "failed_cases": failures, "expected_failures": expected_failures,
                      "unexpected_errors": unexpected_errors, "metrics": metrics}, indent=2))
    # Known errors remain explicitly reported and are not successful diagrams.
    # A changed error or an unexpected success also requires investigation.
    return 1 if changed or unstable or unexpected_errors else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, KeyError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
