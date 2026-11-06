#!/usr/bin/env python3
"""Fail-closed release metadata, semantic version and Go API compatibility checks."""
from __future__ import annotations
import argparse
import datetime as dt
import functools
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[1]
API_TOOL_VERSION = "v0.0.0-20261014143641-f0251eb4aa3e"
BOOTSTRAP_REF = "1a547568e731a76f9b388f05e044da3c90edae0d"
SEMVER = re.compile(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?\Z")

class GateError(Exception):
    pass

@functools.total_ordering
class Version:
    def __init__(self, value):
        self.value = value
        match = SEMVER.fullmatch(value)
        if not match:
            raise GateError(f"invalid semantic version: {value!r}")
        if match.group(5):
            raise GateError(f"Go release versions must be canonical, without build metadata: {value}")
        self.major, self.minor, self.patch = map(int, match.group(1, 2, 3))
        self.pre = match.group(4)
        if self.pre and any(x.isdigit() and len(x) > 1 and x[0] == "0" for x in self.pre.split(".")):
            raise GateError(f"invalid numeric prerelease identifier: {value}")
    def __eq__(self, other):
        return self._cmp(other) == 0
    def __lt__(self, other):
        return self._cmp(other) < 0
    def _cmp(self, other):
        left, right = (self.major, self.minor, self.patch), (other.major, other.minor, other.patch)
        if left != right:
            return (left > right) - (left < right)
        if self.pre == other.pre:
            return 0
        if not self.pre:
            return 1
        if not other.pre:
            return -1
        for a, b in zip(self.pre.split("."), other.pre.split(".")):
            if a == b:
                continue
            if a.isdigit() and b.isdigit():
                return (int(a) > int(b)) - (int(a) < int(b))
            if a.isdigit() != b.isdigit():
                return -1 if a.isdigit() else 1
            return (a > b) - (a < b)
        return (len(self.pre.split(".")) > len(other.pre.split("."))) - (len(self.pre.split(".")) < len(other.pre.split(".")))

def run(args, cwd=ROOT, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise GateError(f"command failed ({result.returncode}): {' '.join(map(str,args))}\n{result.stderr.strip()}")
    return result.stdout

def goenv():
    return {**os.environ, "GOWORK": "off"}

def load_manifest(root=ROOT):
    data = json.loads((root / "release/modules.json").read_text())
    if data.get("schema_version") != 1 or not data.get("modules"):
        raise GateError("release/modules.json: unsupported schema or empty module registry")
    if data.get("bootstrap_ref") != BOOTSTRAP_REF:
        raise GateError("unreleased API baseline is pinned; do not advance it to hide compatibility changes")
    run(["git", "merge-base", "--is-ancestor", BOOTSTRAP_REF, "HEAD"], cwd=root)
    seen_dirs, seen_paths = set(), set()
    for module in data["modules"]:
        directory, path = module["directory"], module["path"]
        if directory in seen_dirs or path in seen_paths:
            raise GateError("duplicate module directory/path")
        if Path(directory).is_absolute() or ".." in Path(directory).parts:
            raise GateError(f"unsafe module directory: {directory}")
        seen_dirs.add(directory); seen_paths.add(path)
        validate_import_version(path, Version(module["version"]))
        info = json.loads(run(["go", "mod", "edit", "-json"], cwd=root / directory, env=goenv()))
        if info["Module"]["Path"] != path:
            raise GateError(f"module path differs from release registry: {directory}")
        if info["Go"] != data["minimum_go"]:
            raise GateError(f"minimum Go mismatch: {directory}")
        core = next((x["Version"] for x in info.get("Require", []) if re.fullmatch(r"github.com/cetrictech/go-platform-kit(?:/v[2-9]\d*)?", x["Path"])), None)
        if core != module.get("core"):
            raise GateError(f"core compatibility matrix mismatch: {directory}: require={core}, registry={module.get('core')}")
        if core:
            Version(core)
        if not (root / module["contracts"]).is_file():
            raise GateError(f"public API contract missing for {directory}")
    discovered = {str(p.parent.relative_to(root)) for p in root.rglob("go.mod") if not any(x in p.parts for x in (".git", ".cache", "vendor"))}
    if discovered != seen_dirs | {"integration"}:
        raise GateError(f"unregistered modules: {sorted(discovered ^ (seen_dirs | {'integration'}))}")
    return data

def validate_import_version(path, version):
    suffix = re.search(r"/v(\d+)$", path)
    if version.major >= 2:
        if not suffix or suffix[1] != str(version.major):
            raise GateError(f"{path}@{version.value}: Go semantic import version requires /v{version.major}")
    elif suffix:
        raise GateError(f"{path}@{version.value}: v0/v1 module must not have a /vN suffix")

def tag_prefix(module):
    directory = module["directory"]
    major = Version(module["version"]).major
    if major >= 2:
        if directory == f"v{major}":
            directory = "."
        elif directory.endswith(f"/v{major}"):
            directory = directory[:-(len(str(major)) + 2)]
    return "" if directory == "." else directory + "/"

def tag_for(module):
    return tag_prefix(module) + module["version"]

def previous_release(module, tags, root=ROOT):
    prefix = tag_prefix(module)
    candidate = Version(module["version"])
    own_tag = tag_for(module)
    head = run(["git", "rev-parse", "HEAD"], cwd=root).strip()
    versions = []
    for tag in tags:
        if not tag.startswith(prefix):
            continue
        name = tag[len(prefix):]
        if "/" in name or not name.startswith("v"):
            continue
        version = Version(name)
        # A pushed candidate tag is the object under test, never its own baseline.
        if tag == own_tag and run(["git", "rev-parse", tag + "^{commit}"], cwd=root).strip() == head:
            continue
        same_line = version.major == candidate.major or candidate.major == 1 and version.major == 0
        if version.pre is None and same_line:
            run(["git", "merge-base", "--is-ancestor", tag + "^{commit}", "HEAD"], cwd=root)
            versions.append((version, tag))
    if not versions:
        return None
    latest = max(versions, key=lambda x: x[0])
    if candidate < latest[0]:
        raise GateError(f"candidate {module['version']} predates latest module release {latest[1]}")
    return latest

def validate_change(module, old_version, incompatible, compatible, migration_present=False, deprecations=()):
    new = Version(module["version"])
    if old_version is None:
        if incompatible:
            raise GateError(f"{module['directory']}: incompatible change against proposed API baseline; review baseline/version policy explicitly")
        return
    old = old_version
    if new < old:
        raise GateError("release versions cannot move backwards")
    if incompatible:
        if old.major >= 1 and new.major == old.major:
            raise GateError(f"{module['directory']}: breaking API change requires a new major module/import path")
        if old.major == 0 and new.major == 0 and new.minor <= old.minor:
            raise GateError(f"{module['directory']}: pre-v1 breaking API change requires a minor version increment")
        if not migration_present:
            raise GateError(f"{module['directory']}: breaking API change requires version-specific migration notes")
        eligible = [d for d in deprecations if d.get("module") == module["path"] and d.get("remove_in") == new.value]
        if not eligible:
            raise GateError(f"{module['directory']}: no prior deprecation/migration ledger for breaking change")
    elif compatible and new.major == old.major and new.minor <= old.minor:
        raise GateError(f"{module['directory']}: additive public API change requires a minor version increment")

def extract_ref(ref, destination, root=ROOT):
    archive = destination / "source.tar"
    with archive.open("wb") as output:
        result = subprocess.run(["git", "archive", "--format=tar", ref], cwd=root, stdout=output, stderr=subprocess.PIPE)
    if result.returncode:
        raise GateError(f"cannot archive compatibility baseline {ref}: {result.stderr.decode()}")
    target = destination / "source"; target.mkdir()
    with tarfile.open(archive) as tar:
        # Python 3.9 ships on some supported developer machines.
        for entry in tar.getmembers():
            path = Path(entry.name)
            if path.is_absolute() or ".." in path.parts or entry.issym() or entry.islnk():
                raise GateError(f"unsafe archive entry: {entry.name}")
        tar.extractall(target)
    return target

def api_check(args):
    manifest = load_manifest()
    tool = args.apidiff or os.environ.get("APIDIFF") or shutil.which("apidiff") or str(ROOT / ".cache/sdk-tools/apidiff")
    if not Path(tool).is_file():
        raise GateError("pinned apidiff missing; run ./scripts/install-api-tool.sh or set APIDIFF")
    build = run(["go", "version", "-m", tool])
    if f"golang.org/x/exp\t{API_TOOL_VERSION}" not in build:
        raise GateError("apidiff does not match the pinned official tool version")
    if run(["git", "rev-parse", "--is-shallow-repository"]).strip() == "true":
        raise GateError("API checks require full tag/history fetch (checkout fetch-depth: 0)")
    tags = run(["git", "tag", "--list"]).splitlines()
    deps = json.loads((ROOT / "release/deprecations.json").read_text())
    validate_deprecations(deps, manifest)
    with tempfile.TemporaryDirectory(prefix="sdk-api-") as temp:
        temp = Path(temp)
        exports = {}
        for module in manifest["modules"]:
            previous = previous_release(module, tags)
            ref = previous[1] if previous else manifest["bootstrap_ref"]
            if not re.fullmatch(r"[0-9a-f]{40}", ref) and previous is None:
                raise GateError("bootstrap_ref must be an immutable full Git commit")
            if ref not in exports:
                folder = temp / str(len(exports)); folder.mkdir()
                exports[ref] = extract_ref(ref, folder)
            old_root = exports[ref]
            old_mod = old_root / module["directory"]
            if not (old_mod / "go.mod").is_file():
                raise GateError(f"{module['directory']}: baseline module missing; initialize its reviewed bootstrap reference explicitly")
            old_file, new_file = temp / "old.api", temp / "new.api"
            old_info = json.loads(run(["go", "mod", "edit", "-json"], cwd=old_mod, env=goenv()))
            for source, output in ((old_mod, old_file), (ROOT / module["directory"], new_file)):
                import_path = old_info["Module"]["Path"] if source == old_mod else module["path"]
                run([tool, "-m", "-w", str(output), import_path], cwd=source, env=goenv())
            if previous:
                check_wire_fixtures(old_mod, ROOT / module["directory"])
            incompatible = run([tool, "-m", "-incompatible", str(old_file), str(new_file)], env=goenv()).strip()
            report = run([tool, "-m", str(old_file), str(new_file)], env=goenv()).strip()
            migration = ROOT / "docs/migrations" / (module["path"].replace("/", "_") + "-" + module["version"] + ".md")
            # Deprecations must already exist in the old source, not be invented alongside removal.
            old_ledger = old_root / "release/deprecations.json"
            prior = json.loads(old_ledger.read_text()).get("deprecations", []) if old_ledger.exists() else []
            validate_change(module, previous[0] if previous else None, bool(incompatible), "Compatible changes:" in report, migration.is_file(), prior)
            print(f"{module['directory']} {module['version']}: baseline={ref}" + (" (unreleased bootstrap)" if previous is None else ""), flush=True)
            if report:
                print(report, flush=True)
    print("Public Go API compatibility and semantic-version checks passed.")

def validate_deprecations(data, manifest):
    if data.get("schema_version") != 1 or not isinstance(data.get("deprecations"), list):
        raise GateError("invalid deprecation ledger")
    paths = {m["path"] for m in manifest["modules"]}
    for item in data["deprecations"]:
        if item.get("module") not in paths or not all(item.get(x) for x in ("symbol", "replacement", "migration_url")):
            raise GateError("incomplete deprecation entry")
        since, removal = Version(item["since"]), Version(item["remove_in"])
        if removal <= since:
            raise GateError("deprecation removal must follow introduction")
        if removal.major == since.major and removal.minor < since.minor + 2:
            raise GateError("deprecations must span at least two minor releases")
        if not (ROOT / item["migration_url"]).is_file():
            raise GateError("deprecation migration document missing")

def is_https(url):
    parsed = urlparse(url)
    return parsed.scheme == "https" and bool(parsed.netloc) and not parsed.username and not parsed.password

def project_identity(value):
    if not is_https(value):
        raise GateError("consumer project must be an HTTPS repository URL")
    url = urlparse(value)
    if url.query or url.fragment or url.port not in (None, 443):
        raise GateError("consumer project URL must identify a repository without query/fragment/alternate port")
    host = url.hostname.lower()
    if host == "www.github.com":
        host = "github.com"
    parts = [p for p in url.path.rstrip("/").removesuffix(".git").split("/") if p]
    if len(parts) < 2 or any(p in (".", "..") or "%" in p for p in parts):
        raise GateError("consumer project URL must identify a repository")
    if host == "github.com" and len(parts) != 2:
        raise GateError("GitHub project URL must be the repository, not a tree/blob/action URL")
    identity = host + "/" + "/".join(parts).lower()
    if identity == "github.com/cetrictech/go-platform-kit" or identity.startswith("github.com/cetrictech/go-platform-kit/"):
        raise GateError("SDK repository is not an external consuming project")
    return identity

def check_wire_fixtures(old_module, new_module):
    for old in old_module.rglob("*-v[0-9]*.json"):
        if "testdata" not in old.parts:
            continue
        relative = old.relative_to(old_module)
        new = new_module / relative
        if not new.is_file() or new.read_bytes() != old.read_bytes():
            raise GateError(f"released wire contract fixture changed or removed: {relative}; preserve supported contracts")

def check_consumers(manifest, modules, path=ROOT / "release/consumers.json", now=None):
    stable = [m for m in modules if Version(m["version"]).major >= 1]
    if not stable:
        return
    data = json.loads(path.read_text())
    if data.get("schema_version") != 1 or not isinstance(data.get("consumers"), list):
        raise GateError("invalid consumer evidence schema")
    now = now or dt.datetime.now(dt.timezone.utc)
    for module in stable:
        projects, go_lines = set(), set()
        for item in data["consumers"]:
            if item.get("modules", {}).get(module["path"]) != module["version"]:
                continue
            try:
                if not re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)", item["tested_at"]):
                    raise ValueError("RFC3339 required")
                tested = dt.datetime.fromisoformat(item["tested_at"].replace("Z", "+00:00"))
            except (KeyError, ValueError) as err:
                raise GateError("invalid consumer evidence timestamp") from err
            if tested.tzinfo is None or tested > now or now - tested > dt.timedelta(days=90):
                raise GateError("consumer evidence must be timezone-aware, recent (90 days), and not future-dated")
            if not is_https(item.get("project_url", "")) or not is_https(item.get("artifact_url", "")):
                raise GateError("consumer evidence requires HTTPS project and immutable CI artifact links")
            identity = project_identity(item["project_url"])
            if not re.fullmatch(r"[0-9a-f]{40}", item.get("revision", "")) or not item.get("reviewer", "").strip():
                raise GateError("consumer evidence requires immutable revision and reviewer")
            checks = item.get("checks", {})
            if set(checks) != {"compatibility", "integration"} or not all(value is True for value in checks.values()):
                raise GateError("consumer compatibility AND integration checks must pass")
            go = re.fullmatch(r"go(\d+\.\d+)\.\d+", item.get("go_version", ""))
            if not go or go[1] not in manifest["supported_go"]:
                raise GateError("consumer evidence must use a supported Go release")
            projects.add(identity); go_lines.add(go[1])
        if len(projects) < 2 or not set(manifest["supported_go"]).issubset(go_lines):
            raise GateError(f"{module['path']}@{module['version']}: stable release requires two real consuming projects with compatibility/integration evidence across supported Go lines")

def metadata_check(args):
    manifest = load_manifest()
    modules = manifest["modules"]
    if args.module:
        modules = [m for m in modules if m["directory"] == args.module]
        if not modules:
            raise GateError("unknown release module")
    tags = run(["git", "tag", "--list"]).splitlines()
    for module in modules:
        previous_release(module, tags)
        if args.release and tag_for(module) in tags:
            raise GateError(f"tag already exists: {tag_for(module)}; releases are immutable")
    validate_deprecations(json.loads((ROOT / "release/deprecations.json").read_text()), manifest)
    check_consumers(manifest, modules)
    print("Module registry, semantic import versions, core matrix and consumer evidence policy passed.")

def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    api = sub.add_parser("api"); api.add_argument("--apidiff")
    meta = sub.add_parser("metadata"); meta.add_argument("--module"); meta.add_argument("--release", action="store_true")
    args = parser.parse_args(argv)
    try:
        (api_check if args.command == "api" else metadata_check)(args)
    except (GateError, OSError, KeyError, ValueError) as err:
        print(f"Release gate failed: {err}", file=sys.stderr)
        return 1
    return 0

if __name__ == "__main__":
    sys.exit(main())
