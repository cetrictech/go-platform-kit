import datetime as dt
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("sdk_release", Path(__file__).parents[1] / "sdk_release.py")
gate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(gate)

class VersionTests(unittest.TestCase):
    def test_semver_precedence(self):
        names = ["v1.0.0-alpha", "v1.0.0-alpha.1", "v1.0.0-alpha.beta", "v1.0.0-beta", "v1.0.0-beta.2", "v1.0.0-beta.11", "v1.0.0-rc.1", "v1.0.0", "v1.0.1"]
        self.assertEqual(sorted(names, key=gate.Version), names)
    def test_noncanonical_go_versions_fail(self):
        for name in ("1.0.0", "v01.0.0", "v1.0.0-01", "v1.0.0+build", "v2.0.0+incompatible", "v1.0"):
            with self.subTest(name=name), self.assertRaises(gate.GateError): gate.Version(name)
    def test_import_path_major_enforcement(self):
        for path,version in (("example.com/sdk", "v2.0.0"),("example.com/sdk/v3","v2.0.0"),("example.com/sdk/v02","v2.0.0"),("example.com/sdk/v1","v1.0.0")):
            with self.subTest(path=path),self.assertRaises(gate.GateError):gate.validate_import_version(path,gate.Version(version))
        gate.validate_import_version("example.com/sdk/v2",gate.Version("v2.1.0"))
    def test_major_subdirectory_tag_prefix(self):
        self.assertEqual(gate.tag_for({"directory":"v2","version":"v2.0.0"}),"v2.0.0")
        self.assertEqual(gate.tag_for({"directory":"database/postgres/v2","version":"v2.0.0"}),"database/postgres/v2.0.0")
    def test_addition_cannot_hide_in_patch(self):
        with self.assertRaises(gate.GateError):gate.validate_change({"directory":".","version":"v1.0.1"},gate.Version("v1.0.0"),False,True)
    def test_stable_break_cannot_hide_in_minor(self):
        with self.assertRaises(gate.GateError):gate.validate_change({"directory":".","version":"v1.1.0"},gate.Version("v1.0.0"),True,False,True,[])
    def test_pre_v1_break_requires_minor_and_prior_migration(self):
        module={"directory":".","path":"example.com/sdk","version":"v0.2.0"}
        with self.assertRaises(gate.GateError):gate.validate_change(module,gate.Version("v0.1.0"),True,False,True,[])
        with self.assertRaises(gate.GateError):gate.validate_change({**module,"version":"v0.1.1"},gate.Version("v0.1.0"),True,False,True,[{"module":"example.com/sdk","remove_in":"v0.1.1"}])
        gate.validate_change(module,gate.Version("v0.1.0"),True,False,True,[{"module":"example.com/sdk","remove_in":"v0.2.0"}])
    def test_bootstrap_break_fails(self):
        with self.assertRaises(gate.GateError):gate.validate_change({"directory":".","version":"v0.1.0"},None,True,False)
    def test_candidate_tag_never_compares_against_itself(self):
        module={"directory":"cache/redis","version":"v0.2.0"}
        with mock.patch.object(gate,"run",return_value="a"*40):
            old=gate.previous_release(module,["cache/redis/v0.1.0","cache/redis/v0.2.0","v9.0.0","grpcx/v0.9.0"])
        self.assertEqual(old[1],"cache/redis/v0.1.0")

class ConsumerTests(unittest.TestCase):
    def setUp(self):
        self.now=dt.datetime(2026,11,5,tzinfo=dt.timezone.utc)
        self.manifest={"supported_go":["1.26","1.27"]}
        self.modules=[{"path":"example.com/sdk","version":"v1.0.0"}]
        self.entries=[self.entry("https://github.com/acme/one","go1.26.10"),self.entry("https://github.com/acme/two","go1.27.3")]
    def entry(self,url,go):
        return {"project_url":url,"revision":"a"*40,"tested_at":"2026-11-04T00:00:00Z","go_version":go,"modules":{"example.com/sdk":"v1.0.0"},"checks":{"compatibility":True,"integration":True},"artifact_url":"https://github.com/acme/one/actions/runs/123","reviewer":"maintainer"}
    def check(self,entries):
        with tempfile.TemporaryDirectory() as folder:
            path=Path(folder)/"consumers.json";path.write_text(json.dumps({"schema_version":1,"consumers":entries}))
            gate.check_consumers(self.manifest,self.modules,path,self.now)
    def test_v1_empty_evidence_is_blocked(self):
        with self.assertRaises(gate.GateError):self.check([])
    def test_valid_reviewed_evidence(self):self.check(self.entries)
    def test_aliases_are_one_project(self):
        self.entries[1]["project_url"]="https://www.github.com/ACME/ONE.git/"
        with self.assertRaises(gate.GateError):self.check(self.entries)
    def test_sdk_repo_and_subpath_are_not_consumer(self):
        for url in ("https://github.com/cetrictech/go-platform-kit/","https://github.com/CetricTech/Go-Platform-Kit.git","https://github.com/cetrictech/go-platform-kit/tree/main"):
            self.entries[0]["project_url"]=url
            with self.subTest(url=url),self.assertRaises(gate.GateError):self.check(self.entries)
    def test_stale_future_or_naive_evidence_fails(self):
        for stamp in ("2025-01-01T00:00:00Z","2027-01-01T00:00:00Z","2026-11-04T00:00:00","2026-11-04 00:00:00Z"):
            self.entries[0]["tested_at"]=stamp
            with self.subTest(stamp=stamp),self.assertRaises(gate.GateError):self.check(self.entries)
    def test_boolean_attestations_are_not_truthy_numbers(self):
        self.entries[0]["checks"]["compatibility"]=1
        with self.assertRaises(gate.GateError):self.check(self.entries)
    def test_failed_consumer_and_missing_revision_fail(self):
        self.entries[0]["checks"]["integration"]=False
        with self.assertRaises(gate.GateError):self.check(self.entries)
        self.entries[0]["checks"]["integration"]=True;self.entries[0]["revision"]="main"
        with self.assertRaises(gate.GateError):self.check(self.entries)
    def test_missing_supported_go_line_fails(self):
        self.entries[1]["go_version"]="go1.26.10"
        with self.assertRaises(gate.GateError):self.check(self.entries)

class WireTests(unittest.TestCase):
    def test_fixture_cannot_change_with_implementation(self):
        with tempfile.TemporaryDirectory() as folder:
            old,new=Path(folder)/"old",Path(folder)/"new"
            for p in (old,new):(p/"response/testdata").mkdir(parents=True)
            path="response/testdata/wire-v0.json"
            (old/path).write_text('{"code":"NOT_FOUND"}')
            (new/path).write_text('{"code":"MISSING"}')
            with self.assertRaises(gate.GateError):gate.check_wire_fixtures(old,new)
            (new/path).write_bytes((old/path).read_bytes());gate.check_wire_fixtures(old,new)

if __name__=="__main__":unittest.main()
