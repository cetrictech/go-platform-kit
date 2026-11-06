import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("gate", Path(__file__).with_name("check-benchmarks.py"))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class BenchmarkGateTests(unittest.TestCase):
    def setUp(self):
        self.metadata = {"schema": 1, "go_version": "go1.27.3", "goos": "darwin", "goarch": "arm64", "cpu": "test CPU", "gomaxprocs": 2, "race": False, "suite": "core-v1", "benchtime": "200ms", "commit": "a" * 40}
        self.text = "goos: darwin\ngoarch: arm64\ncpu: test CPU\npkg: example.test/pkg\n" + "BenchmarkPath-2 1000 100 ns/op 20 B/op 2 allocs/op\n" * 10
        self.results = gate.parse_benchmarks(self.text)
        self.baseline = gate.make_baseline(self.metadata, self.results)

    def test_passes_identical_and_improved_results(self):
        self.assertEqual(gate.compare(self.baseline, self.metadata, self.results), [])
        improved = gate.parse_benchmarks(self.text.replace("100 ns/op", "80 ns/op"))
        self.assertEqual(gate.compare(self.baseline, self.metadata, improved), [])

    def test_rejects_regressions_for_every_metric(self):
        for before, after in [("100 ns/op", "121 ns/op"), ("20 B/op", "23 B/op"), ("2 allocs/op", "3 allocs/op")]:
            with self.subTest(metric=before):
                self.assertTrue(gate.compare(self.baseline, self.metadata, gate.parse_benchmarks(self.text.replace(before, after))))

    def test_rejects_missing_case_samples_and_metrics(self):
        with self.assertRaises(gate.EvidenceError):
            gate.compare(self.baseline, self.metadata, {})
        fewer = gate.parse_benchmarks("pkg: example.test/pkg\nBenchmarkPath-2 10 100 ns/op 20 B/op 2 allocs/op\n")
        with self.assertRaises(gate.EvidenceError):
            gate.compare(self.baseline, self.metadata, fewer)
        with self.assertRaises(gate.EvidenceError):
            gate.parse_benchmarks(self.text.replace(" 2 allocs/op", ""))

    def test_rejects_environment_mismatch_and_race(self):
        for field, value in [("cpu", "other"), ("go_version", "go1.26.10"), ("gomaxprocs", 4), ("race", True), ("benchtime", "1s")]:
            metadata = {**self.metadata, field: value}
            with self.subTest(field=field), self.assertRaises(gate.EvidenceError):
                gate.compare(self.baseline, metadata, self.results)

    def test_rejects_invalid_evidence_or_weakened_policy(self):
        for text in ["", "BenchmarkBad 10 20 ns/op", self.text + "FAIL\n", self.text.replace("100 ns/op", "NaN ns/op")]:
            with self.subTest(text=text), self.assertRaises(gate.EvidenceError):
                gate.parse_benchmarks(text)
        for key, value in [("minimum_samples", 1), ("max_relative_mad", 1), ("max_regression_fraction", {"ns/op": 1, "B/op": 0, "allocs/op": 0})]:
            baseline = {**self.baseline, key: value}
            with self.subTest(key=key), self.assertRaises(gate.EvidenceError):
                gate.compare(baseline, self.metadata, self.results)
        modified = copy.deepcopy(self.baseline)
        modified["benchmarks"]["example.test/pkg/BenchmarkPath"]["ns/op"]["median"] = 1000
        with self.assertRaises(gate.EvidenceError):
            gate.compare(modified, self.metadata, self.results)

    def test_preserves_package_and_subbenchmark_identity(self):
        parsed = gate.parse_benchmarks("pkg: one.test/a\nBenchmarkSame/a-2 2 10 ns/op 0 B/op 0 allocs/op\npkg: two.test/b\nBenchmarkSame/a-2 2 10 ns/op 0 B/op 0 allocs/op\n")
        self.assertEqual(set(parsed), {"one.test/a/BenchmarkSame/a", "two.test/b/BenchmarkSame/a"})

    def test_unstable_repeated_samples_fail(self):
        noisy = copy.deepcopy(self.results)
        noisy["example.test/pkg/BenchmarkPath"]["ns/op"] = [50, 150] * 5
        with self.assertRaises(gate.EvidenceError):
            gate.compare(self.baseline, self.metadata, noisy)

    def test_log_headers_and_cpu_suffix_are_bound_to_metadata(self):
        gate.validate_log_metadata(self.text, self.metadata)
        for before, after in [("goos: darwin", "goos: linux"), ("goarch: arm64", "goarch: amd64"), ("cpu: test CPU", "cpu: other CPU"), ("BenchmarkPath-2", "BenchmarkPath-4")]:
            with self.subTest(field=before), self.assertRaises(gate.EvidenceError):
                gate.validate_log_metadata(self.text.replace(before, after), self.metadata)
        with self.assertRaises(gate.EvidenceError):
            gate.validate_log_metadata(self.text.replace("cpu: test CPU\n", ""), self.metadata)

    def test_release_evidence_requires_clean_both_sides(self):
        with self.assertRaises(gate.EvidenceError):
            gate.compare(self.baseline, self.metadata, self.results, release=True)
        clean = {**self.metadata, "source_dirty": False}
        baseline = gate.make_baseline(clean, self.results)
        self.assertEqual(gate.compare(baseline, clean, self.results, release=True), [])
        with self.assertRaises(gate.EvidenceError):
            gate.compare(baseline, {**clean, "source_dirty": True}, self.results, release=True)

    def test_zero_allocation_growth_fails(self):
        samples = gate.parse_benchmarks(self.text.replace("2 allocs/op", "0 allocs/op"))
        baseline = gate.make_baseline(self.metadata, samples)
        self.assertTrue(gate.compare(baseline, self.metadata, self.results))


if __name__ == "__main__":
    unittest.main()
