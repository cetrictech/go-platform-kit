#!/usr/bin/env python3
"""Fail closed on missing, incomparable, noisy, or regressed Go benchmark evidence.

Normal use compares --input and --metadata to a reviewed --baseline. --record
creates a candidate baseline explicitly and never runs as part of comparison.
"""
from __future__ import annotations

import argparse
import json
import math
import re
import statistics
import sys
from pathlib import Path

METRICS = ("ns/op", "B/op", "allocs/op")
ENVIRONMENT = ("go_version", "goos", "goarch", "cpu", "gomaxprocs", "race", "suite", "benchtime")
MINIMUM_SAMPLES = 7
MAX_RELATIVE_MAD = 0.15
THRESHOLDS = {"ns/op": 0.20, "B/op": 0.10, "allocs/op": 0.0}
BENCHMARK = re.compile(r"^(Benchmark\S+)\s+(\d+)\s+(.+)$")
CPU_SUFFIX = re.compile(r"-\d+$")


class EvidenceError(ValueError):
    pass


def read_json(path: Path) -> dict:
    try:
        value = json.loads(path.read_text())
    except (OSError, ValueError) as error:
        raise EvidenceError(f"cannot read {path}: {error}") from error
    if not isinstance(value, dict):
        raise EvidenceError(f"{path}: expected a JSON object")
    return value


def validate_metadata(metadata: dict) -> None:
    if metadata.get("schema") != 1:
        raise EvidenceError("unsupported metadata schema")
    for field in ENVIRONMENT:
        if field not in metadata or metadata[field] is None or metadata[field] == "":
            raise EvidenceError(f"missing environment metadata: {field}")
    if type(metadata["gomaxprocs"]) is not int or metadata["gomaxprocs"] < 1:
        raise EvidenceError("gomaxprocs must be a positive integer")
    if metadata["race"] is not False:
        raise EvidenceError("performance evidence must be collected without race instrumentation")
    if not isinstance(metadata.get("commit"), str) or not re.fullmatch(r"[a-f0-9]{40,64}", metadata["commit"]):
        raise EvidenceError("full source commit is required in metadata")


def parse_benchmarks(text: str) -> dict[str, dict[str, list[float]]]:
    package = None
    results: dict[str, dict[str, list[float]]] = {}
    for line in text.splitlines():
        if line.startswith("FAIL") or line.startswith("--- FAIL"):
            raise EvidenceError("benchmark process contains a failure")
        if line.startswith("pkg: "):
            package = line[5:].strip()
        if not line.startswith("Benchmark"):
            continue
        match = BENCHMARK.match(line)
        if not match or not package:
            raise EvidenceError(f"malformed benchmark or missing package: {line}")
        name, iterations, fields = match.groups()
        if int(iterations) < 1:
            raise EvidenceError(f"no benchmark iterations: {line}")
        fields = fields.split()
        if len(fields) % 2:
            raise EvidenceError(f"unpaired benchmark measurements: {line}")
        values = {}
        for offset in range(0, len(fields), 2):
            try:
                number = float(fields[offset])
            except ValueError as error:
                raise EvidenceError(f"invalid measurement: {line}") from error
            if not math.isfinite(number) or number < 0:
                raise EvidenceError(f"non-finite or negative measurement: {line}")
            unit = fields[offset + 1]
            if unit in values:
                raise EvidenceError(f"duplicate measurement unit: {line}")
            values[unit] = number
        if set(METRICS) - values.keys() or values["ns/op"] <= 0:
            raise EvidenceError(f"missing required benchmark measurements: {line}")
        identity = package + "/" + CPU_SUFFIX.sub("", name)
        case = results.setdefault(identity, {metric: [] for metric in METRICS})
        for metric in METRICS:
            case[metric].append(values[metric])
    if not results:
        raise EvidenceError("no benchmark measurements found")
    return results


def validate_log_metadata(text: str, metadata: dict) -> None:
    """Bind supplied metadata to the measured log, not an unrelated run."""
    validate_metadata(metadata)
    for field in ("goos", "goarch", "cpu"):
        prefix = field + ": "
        values = {line[len(prefix):].strip() for line in text.splitlines() if line.startswith(prefix)}
        if values != {metadata[field]}:
            raise EvidenceError(f"benchmark log {field} header does not match metadata")
    for line in text.splitlines():
        if not line.startswith("Benchmark"):
            continue
        match = BENCHMARK.match(line)
        if not match:
            raise EvidenceError(f"malformed benchmark: {line}")
        suffix = CPU_SUFFIX.search(match[1])
        cpu_count = int(suffix[0][1:]) if suffix else 1
        if cpu_count != metadata["gomaxprocs"]:
            raise EvidenceError("benchmark CPU suffix does not match gomaxprocs metadata")


def summarize(samples: list[float], name: str, minimum: int, max_mad: float) -> dict:
    if len(samples) < minimum:
        raise EvidenceError(f"{name}: {len(samples)} samples; at least {minimum} required")
    median = statistics.median(samples)
    mad = statistics.median(abs(sample - median) for sample in samples)
    if (median == 0 and mad != 0) or (median > 0 and mad / median > max_mad):
        raise EvidenceError(f"{name}: unstable samples (median={median:g}, MAD={mad:g}); recollect on an idle host")
    return {"median": median, "mad": mad, "samples": samples}


def make_baseline(metadata: dict, results: dict) -> dict:
    validate_metadata(metadata)
    cases = {}
    for identity, measurements in sorted(results.items()):
        cases[identity] = {metric: summarize(values, identity + " " + metric, MINIMUM_SAMPLES, MAX_RELATIVE_MAD) for metric, values in measurements.items()}
    return {"schema": 1, "metadata": metadata, "minimum_samples": MINIMUM_SAMPLES,
            "max_relative_mad": MAX_RELATIVE_MAD, "max_regression_fraction": THRESHOLDS,
            "benchmarks": cases}


def compare(baseline: dict, metadata: dict, results: dict, release: bool = False) -> list[str]:
    validate_metadata(metadata)
    if baseline.get("schema") != 1 or not isinstance(baseline.get("benchmarks"), dict):
        raise EvidenceError("unsupported or incomplete performance baseline")
    validate_metadata(baseline.get("metadata", {}))
    if release and (metadata.get("source_dirty") is not False or baseline["metadata"].get("source_dirty") is not False):
        raise EvidenceError("release performance evidence requires clean baseline and candidate source trees")
    mismatches = [field for field in ENVIRONMENT if metadata[field] != baseline["metadata"][field]]
    if mismatches:
        raise EvidenceError("ineligible comparison environment: " + ", ".join(mismatches))
    minimum = baseline.get("minimum_samples")
    if type(minimum) is not int or minimum < MINIMUM_SAMPLES:
        raise EvidenceError("baseline sample policy is weaker than the required minimum")
    max_mad = baseline.get("max_relative_mad")
    if type(max_mad) not in (int, float) or not math.isfinite(max_mad) or not 0 <= max_mad <= MAX_RELATIVE_MAD:
        raise EvidenceError("baseline dispersion policy is invalid")
    thresholds = baseline.get("max_regression_fraction")
    if not isinstance(thresholds, dict) or set(thresholds) != set(METRICS):
        raise EvidenceError("missing benchmark regression policy")
    for metric, fraction in thresholds.items():
        if type(fraction) not in (int, float) or not math.isfinite(fraction) or not 0 <= fraction <= THRESHOLDS[metric]:
            raise EvidenceError(f"baseline threshold is invalid or weakened: {metric}")
    expected = baseline["benchmarks"]
    if not expected:
        raise EvidenceError("empty performance baseline")
    missing = set(expected) - results.keys()
    if missing:
        raise EvidenceError("missing benchmark cases: " + ", ".join(sorted(missing)))
    extra = results.keys() - set(expected)
    if extra:
        raise EvidenceError("new benchmark cases require explicit baseline review: " + ", ".join(sorted(extra)))
    failures = []
    for identity, measurements in sorted(expected.items()):
        if not isinstance(measurements, dict) or set(measurements) != set(METRICS):
            raise EvidenceError(f"incomplete baseline metrics: {identity}")
        for metric in METRICS:
            reference = measurements[metric]
            if not isinstance(reference, dict) or not isinstance(reference.get("samples"), list):
                raise EvidenceError(f"baseline raw samples missing: {identity} {metric}")
            samples = reference["samples"]
            if any(type(value) not in (int, float) or not math.isfinite(value) or value < 0 for value in samples):
                raise EvidenceError(f"invalid baseline raw samples: {identity} {metric}")
            previous = summarize(samples, identity + " baseline " + metric, minimum, max_mad)
            if reference.get("median") != previous["median"] or reference.get("mad") != previous["mad"]:
                raise EvidenceError(f"baseline summary differs from raw samples: {identity} {metric}")
            current = summarize(results[identity][metric], identity + " " + metric, minimum, max_mad)
            limit = previous["median"] * (1 + thresholds[metric])
            if current["median"] > limit:
                failures.append(f"{identity} {metric}: {current['median']:g} exceeds {limit:g} (baseline {previous['median']:g})")
    return failures


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--metadata", required=True, type=Path)
    parser.add_argument("--baseline", type=Path)
    parser.add_argument("--candidate", type=Path, help="new candidate output path for explicit --record")
    parser.add_argument("--record", action="store_true", help="write a candidate baseline; requires explicit maintainer review")
    parser.add_argument("--release", action="store_true", help="require clean source trees in baseline and candidate metadata")
    arguments = parser.parse_args(argv)
    try:
        metadata = read_json(arguments.metadata)
        text = arguments.input.read_text()
        validate_log_metadata(text, metadata)
        results = parse_benchmarks(text)
        if arguments.record:
            if arguments.candidate is None or arguments.baseline is not None:
                raise EvidenceError("--record requires --candidate and prohibits --baseline")
            if arguments.release and metadata.get("source_dirty") is not False:
                raise EvidenceError("release performance evidence requires a clean source tree")
            baseline = make_baseline(metadata, results)
            with arguments.candidate.open("x") as output:
                output.write(json.dumps(baseline, indent=2, sort_keys=True) + "\n")
            print(f"Recorded candidate for {len(results)} cases; review metadata, samples and policy before adoption")
            return 0
        if arguments.baseline is None or arguments.candidate is not None:
            raise EvidenceError("comparison requires --baseline and prohibits --candidate")
        failures = compare(read_json(arguments.baseline), metadata, results, release=arguments.release)
        if failures:
            raise EvidenceError("performance regressions:\n" + "\n".join(failures))
        print(f"PASS: {len(results)} benchmark cases; environment and repeated medians match policy")
        return 0
    except (EvidenceError, OSError) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
