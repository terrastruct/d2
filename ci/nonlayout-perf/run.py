#!/usr/bin/env python3
"""Measure the real E2E corpus with layout excluded, without editing its tests."""

import argparse
import collections
import hashlib
import json
import os
from pathlib import Path
import resource
import statistics
import sys
import subprocess
import time

sys.dont_write_bytecode = True

from overlay import HERE, git, prepare


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def execute(command, cwd, env, log):
    before = resource.getrusage(resource.RUSAGE_CHILDREN)
    start = time.perf_counter()
    with log.open("w") as output:
        result = subprocess.run(command, cwd=cwd, env=env, stdout=output, stderr=subprocess.STDOUT)
    wall = time.perf_counter() - start
    after = resource.getrusage(resource.RUSAGE_CHILDREN)
    if result.returncode:
        raise RuntimeError(f"Command failed ({result.returncode}); see {log}")
    return {
        "wall_seconds": wall,
        "user_seconds": after.ru_utime - before.ru_utime,
        "system_seconds": after.ru_stime - before.ru_stime,
    }


def run(args):
    repo = args.repo.resolve()
    output = args.out.resolve()
    output.mkdir(parents=True, exist_ok=True)
    if list(output.glob("run-*.json")):
        raise ValueError(f"Refusing to overwrite previous results in {output}; use a fresh output directory")
    base_ref = git(repo, "rev-parse", args.base_ref).strip() if args.base_ref else None
    if base_ref:
        for name in ("go.mod", "go.sum"):
            if git(repo, "show", f"{base_ref}:{name}") != (repo / name).read_text():
                raise ValueError(f"{name} differs from the baseline; use a separate baseline worktree")
        changed = set(git(repo, "diff", "--name-only", base_ref).splitlines())
        changed.update(git(repo, "ls-files", "--others", "--exclude-standard").splitlines())
        assets = sorted(path for path in changed
                        if not path.endswith(".go") and not path.startswith("ci/nonlayout-perf/"))
        if assets:
            raise ValueError(f"Non-Go inputs differ from baseline; use a separate baseline worktree: {assets}")
    mapping = prepare(repo, output / "overlay", base_ref)
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": mapping}, indent=2) + "\n")
    binary = output / ("e2e.test" if args.mode == "phases" else "render")
    if args.mode == "phases":
        command = ["go", "test", "-c", "-overlay", str(overlay), "-o", str(binary), "./e2etests"]
    else:
        runner = output / "render.go"
        runner.write_text((HERE / "render.go.txt").read_text())
        command = ["go", "build", "-overlay", str(overlay), "-o", str(binary), str(runner)]
    subprocess.run(command, cwd=repo, check=True)
    provenance = {
        "source_ref": base_ref,
        "worktree_head": git(repo, "rev-parse", "HEAD").strip(),
        "worktree_tracked_diff_sha256": hashlib.sha256(git(repo, "diff", "--binary", "HEAD").encode()).hexdigest(),
        "go_version": subprocess.check_output(["go", "version"], text=True).strip(),
        "binary_sha256": sha256(binary),
        "harness_sha256": {p.name: sha256(p) for p in HERE.iterdir() if p.is_file()},
        "gomaxprocs": args.gomaxprocs,
        "worktree_untracked_go_sha256": {
            path: sha256(repo / path)
            for path in git(repo, "ls-files", "--others", "--exclude-standard", "--", "*.go").splitlines()
        },
    }
    provenance["runs"] = args.runs
    provenance["passes"] = getattr(args, "passes", None)
    provenance["mode"] = args.mode
    provenance["test_filter"] = getattr(args, "test", None)
    if args.mode == "render":
        provenance["corpus_sha256"] = {path.name: sha256(path) for path in sorted(args.corpus.glob("*.json"))}
        provenance["oracle_sha256"] = sha256(args.oracle)
    (output / "provenance.json").write_text(json.dumps(provenance, indent=2) + "\n")
    for index in range(args.runs):
        result_path = output / f"run-{index}.json"
        env = dict(os.environ, GOMAXPROCS=str(args.gomaxprocs), CI="1")
        # Do not inherit settings that change test verification or add capture work.
        for key in ("SKIP_SVG_CHECK", "TESTDATA_ACCEPT", "TA", "NO_DIFF", "ND", "D2_NONLAYOUT_CAPTURE", "D2_NONLAYOUT_PHASE_OUT"):
            env.pop(key, None)
        command = [str(binary)]
        if args.mode == "phases":
            cwd = repo / "e2etests"
            env["D2_NONLAYOUT_PHASE_OUT"] = str(result_path)
            if args.capture and index == 0:
                env["D2_NONLAYOUT_CAPTURE"] = str(output / "corpus")
            command += ["-test.run", args.test, "-test.parallel", "1"]
            if args.profile:
                command += ["-test.cpuprofile", str(output / f"run-{index}.cpu.pprof")]
        else:
            cwd = repo
            command += ["-corpus", str(args.corpus.resolve()), "-out", str(result_path), "-repeat", str(args.passes)]
            if args.profile:
                command += ["-cpu-profile", str(output / f"run-{index}.cpu.pprof")]
        process = execute(command, cwd, env, output / f"run-{index}.log")
        data = json.loads(result_path.read_text())
        if args.mode == "render":
            oracle = fingerprints(json.loads(args.oracle.read_text()))
            expected = {key: value for key, value in oracle.items() if key[1] == "svg"}
            if fingerprints(data) != expected:
                raise ValueError("Render replay differs from the original E2E SVG output set")
        data["process"] = process
        data["profiled"] = args.profile
        data["requested_passes"] = getattr(args, "passes", None)
        result_path.write_text(json.dumps(data, indent=2) + "\n")
        print(result_path, json.dumps(process), flush=True)


def phase_summary(data):
    totals = collections.Counter()
    counts = collections.Counter()
    for event in data["events"].values():
        totals[event["phase"]] += event["nanoseconds"] / 1e9
        counts[event["phase"]] += event["count"]
    product = totals["compile_total"] - totals["layout_excluded"] + totals["svg_render"] + totals["animation"]
    setup = sum(totals[k] for k in ("ruler", "serde_compiler", "serde_dimensions", "serde_serialize", "serde_deserialize", "serde_json_compare"))
    other = sum(totals[k] for k in ("xml_verify", "ascii_extended", "ascii_standard"))
    return {
        "product_nonlayout_seconds": product,
        "setup_serde_seconds": setup,
        "all_timed_nonlayout_seconds": product + setup + other,
        "phase_seconds": dict(totals),
        "phase_counts": dict(counts),
    }


def fingerprints(data):
    if "events" in data:
        rows = [(x["case"], x["kind"], x["sha256"], x["bytes"]) for x in data["hashes"]]
        groups = [rows]
    else:
        cases = data["cases"]
        passes = data["passes"]
        if not isinstance(cases, int) or cases < 1 or not passes:
            raise ValueError("Empty or invalid render metadata")
        ids = [row["pass"] for row in passes]
        if ids != list(range(len(passes))):
            raise ValueError("Missing, duplicate, or unordered pass metadata")
        if data.get("requested_passes") not in (None, len(passes)):
            raise ValueError("A requested render pass is missing")
        if len(data["results"]) != cases * len(passes):
            raise ValueError("Render result count disagrees with case/pass metadata")
        if {row["pass"] for row in data["results"]} != set(ids):
            raise ValueError("Render result pass IDs disagree with metadata")
        groups = []
        for metadata in passes:
            rows = [row for row in data["results"] if row["pass"] == metadata["pass"]]
            if len(rows) != cases:
                raise ValueError("A render pass has missing or additional cases")
            for field in ("render_ns", "animation_ns"):
                if metadata[field] != sum(row[field] for row in rows):
                    raise ValueError(f"Render pass {field} disagrees with per-case results")
            if any(row["input_mutated"] for row in rows):
                raise ValueError("A render changed its input")
            groups.append([(row["case"], "svg", row["sha256"], row["bytes"]) for row in rows])
    result = None
    for rows in groups:
        current = {(case, kind): (digest, size) for case, kind, digest, size in rows}
        if len(current) != len(rows) or not rows:
            raise ValueError("Duplicate or empty fingerprint set")
        if result is not None and current != result:
            raise ValueError("Repeated passes changed output")
        result = current
    return result


def percentile(values, fraction):
    values = sorted(values)
    position = (len(values) - 1) * fraction
    lower = int(position)
    upper = min(lower + 1, len(values) - 1)
    return values[lower] + (values[upper] - values[lower]) * (position - lower)


def compare(args):
    variants = [[json.loads(path.read_text()) for path in paths] for paths in (args.baseline, args.candidate)]
    expected = fingerprints(variants[0][0])
    phases = "events" in variants[0][0]
    expected_counts = None
    if phases:
        expected_counts = {key: value["count"] for key, value in variants[0][0]["events"].items()}
    for data in sum(variants, []):
        if ("events" in data) != phases:
            raise ValueError("Compare phase files together or render files together")
        if phases:
            counts = {key: value["count"] for key, value in data["events"].items()}
            if counts != expected_counts:
                raise ValueError("Measured case/phase invocation counts changed")
        actual = fingerprints(data)
        if actual != expected:
            changed = sorted(key for key in expected.keys() | actual.keys() if expected.get(key) != actual.get(key))
            raise ValueError(f"Output mismatch: {changed}")
    report = {"fingerprints_equal": len(expected), "profiled": any(x.get("profiled", False) for x in sum(variants, []))}
    if phases:
        summary = [[phase_summary(x) for x in group] for group in variants]
        report["baseline"], report["candidate"] = summary
        report["ratios"] = {
            key: statistics.median(x[key] for x in summary[0]) / statistics.median(x[key] for x in summary[1])
            for key in ("product_nonlayout_seconds", "setup_serde_seconds", "all_timed_nonlayout_seconds")
        }
    else:
        summaries = []
        case_times = []
        for group in variants:
            totals = []
            cases = collections.defaultdict(list)
            for data in group:
                totals += [(x["render_ns"] + x["animation_ns"]) / 1e9 for x in data["passes"]]
                for row in data["results"]:
                    cases[row["case"]].append(row["render_ns"] + row["animation_ns"])
            summaries.append(totals)
            case_times.append({key: statistics.median(values) for key, values in cases.items()})
        ratios = {key: case_times[0][key] / case_times[1][key] for key in case_times[0]}
        report["baseline_seconds"], report["candidate_seconds"] = summaries
        report["aggregate_ratio"] = statistics.median(summaries[0]) / statistics.median(summaries[1])
        report["case_ratio_distribution"] = {"p10": percentile(ratios.values(), .1), "median": statistics.median(ratios.values()), "p90": percentile(ratios.values(), .9)}
        if args.threshold is not None:
            report["case_ratio_threshold"] = {"threshold": args.threshold, "count": sum(x >= args.threshold for x in ratios.values()), "cases": len(ratios)}
        report["case_ratios"] = ratios
    report["process"] = [[x.get("process") for x in group] for group in variants]
    print(json.dumps(report, indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    modes = parser.add_subparsers(dest="mode", required=True)
    for name in ("phases", "render"):
        command = modes.add_parser(name)
        command.add_argument("--repo", type=Path, default=HERE.parents[1])
        command.add_argument("--out", type=Path, required=True)
        sources = command.add_mutually_exclusive_group(required=True)
        sources.add_argument("--base-ref", help="Explicit immutable baseline ref; overlaid onto this worktree")
        sources.add_argument("--current", action="store_true", help="Measure current worktree sources")
        command.add_argument("--runs", type=int, default=1)
        command.add_argument("--gomaxprocs", type=int, default=1)
        command.add_argument("--profile", action="store_true", help="Diagnostic only; do not use these timings for final claims")
        if name == "phases":
            command.add_argument("--capture", action="store_true")
            command.add_argument("--test", default="^TestE2E$", help="Optional smoke-test filter; full corpus is the default")
        else:
            command.add_argument("--corpus", type=Path, required=True)
            command.add_argument("--oracle", type=Path, required=True, help="Phase JSON from the E2E run that captured the corpus")
            command.add_argument("--passes", type=int, default=1)
    command = modes.add_parser("compare")
    command.add_argument("--baseline", type=Path, nargs="+", required=True)
    command.add_argument("--candidate", type=Path, nargs="+", required=True)
    command.add_argument("--threshold", type=float, help="Optional per-diagram speedup threshold to count")
    args = parser.parse_args()
    if args.mode == "compare":
        compare(args)
    else:
        if args.runs < 1 or args.gomaxprocs < 1 or getattr(args, "passes", 1) < 1:
            parser.error("runs, passes, and gomaxprocs must be positive")
        run(args)


if __name__ == "__main__":
    main()
