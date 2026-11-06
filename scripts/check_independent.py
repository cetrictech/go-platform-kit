#!/usr/bin/env python3
"""Build/test exported modules and isolated one-module consumers outside go.work.

Local replacements are development linkage, not evidence that a version has
been published or that a real external consuming project has qualified v1.
"""
from __future__ import annotations
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

OPTIONAL_DRIVERS = re.compile(r"(?:jackc/pgx|go-sql-driver/mysql|redis/go-redis|twmb/franz-go|IBM/sarama|google\.golang\.org/grpc|spiffe/go-spiffe|go\.opentelemetry\.io|golang-jwt|aws/aws-sdk-go)")


def run(command: list[str], cwd: Path, environment: dict[str, str], capture: bool = False) -> str:
    print("+ " + " ".join(command) + " [" + str(cwd) + "]", flush=True)
    result = subprocess.run(command, cwd=cwd, env=environment, check=True, text=True,
                            stdout=subprocess.PIPE if capture else None)
    return result.stdout or ""


def export_sources(source: Path, destination: Path) -> None:
    files = subprocess.check_output(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], cwd=source)
    for raw in files.split(b"\0"):
        if not raw:
            continue
        relative = Path(os.fsdecode(raw))
        if relative.is_absolute() or ".." in relative.parts:
            raise ValueError("unsafe export path")
        if relative.parts[0] in (".git", ".agents", ".codex") or relative.name in ("go.work", "go.work.sum"):
            continue
        original = source / relative
        if not original.exists():
            # A tracked file deleted in the current change is not part of the export.
            continue
        if original.is_symlink():
            raise ValueError("source export rejects symlinks: " + str(relative))
        if not original.is_file():
            continue
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(original, target)


def public_packages(module: Path, environment: dict[str, str]) -> list[str]:
    lines = run(["go", "list", "-f", "{{.ImportPath}} {{.Name}}", "./..."], module, environment, capture=True).splitlines()
    result = []
    for line in lines:
        name, package = line.split()
        if "/internal/" not in name and package != "main":
            result.append(name)
    if not result:
        raise ValueError("module has no public library packages: " + str(module))
    return result


def consumer(parent: Path, index: int, specification: dict, exported: Path, core: dict,
             environment: dict[str, str], packages: list[str]) -> None:
    destination = parent / ("consumer-" + str(index))
    destination.mkdir()
    module = specification["path"]
    replacement = exported / specification["directory"]
    lines = ["module consumer.example/independent", "", "go " + core["minimum_go"], "", "require " + module + " " + specification["version"], "", "replace " + module + " => " + str(replacement)]
    if specification.get("core") is not None:
        lines.append("replace " + core["path"] + " => " + str(exported))
    (destination / "go.mod").write_text("\n".join(lines) + "\n")
    sums = replacement / "go.sum"
    if sums.exists():
        shutil.copyfile(sums, destination / "go.sum")
    imports = "\n".join(' _ "' + package + '"' for package in packages)
    (destination / "imports.go").write_text('package consumer\nimport (\n' + imports + '\n)\n')
    (destination / "imports_test.go").write_text('package consumer\nimport "testing"\nfunc TestIndependentImport(t *testing.T) {}\n')
    # Resolve only the consumer build graph. go mod tidy additionally loads
    # unrelated third-party dependency tests, which this import check does not run.
    run(["go", "build", "-mod=mod", "./..."], destination, environment)
    run(["go", "build", "./..."], destination, environment)
    run(["go", "test", "-count=1", "./..."], destination, environment)


def check_logger_graph(parent: Path, exported: Path, core: dict, environment: dict[str, str]) -> None:
    destination = parent / "logger-only"
    destination.mkdir()
    (destination / "go.mod").write_text("module consumer.example/logger\n\ngo " + core["minimum_go"] + "\n\nrequire " + core["path"] + " " + core["version"] + "\nreplace " + core["path"] + " => " + str(exported) + "\n")
    (destination / "main.go").write_text('package main\nimport "' + core["path"] + '/logger"\nfunc main(){ _ = logger.Config{} }\n')
    shutil.copyfile(exported / "go.sum", destination / "go.sum")
    run(["go", "mod", "tidy"], destination, {**environment, "GOFLAGS": ""})
    graph = run(["go", "list", "-m", "all"], destination, environment, capture=True)
    if OPTIONAL_DRIVERS.search(graph):
        raise ValueError("optional driver entered logger-only consumer graph:\n" + graph)
    run(["go", "build", "./..."], destination, environment)
    print("PASS: logger-only graph excludes optional adapters, telemetry and JWT", flush=True)


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--online", action="store_true", help="allow configured Go proxies; default proves independence using cached dependencies")
    parser.add_argument("--module", help="check one manifest module directory (or integration) during development")
    arguments = parser.parse_args(argv)
    source = Path(__file__).resolve().parents[1]
    try:
        manifest = json.loads((source / "release/modules.json").read_text())
        modules = manifest["modules"]
        core = next(module for module in modules if module["directory"] == ".")
        core = {**core, "minimum_go": manifest["minimum_go"]}
        selected = [module for module in modules if arguments.module in (None, module["directory"])]
        if arguments.module is not None and not selected and arguments.module != "integration":
            raise ValueError("unknown module directory: " + arguments.module)
        with tempfile.TemporaryDirectory(prefix="sdk-independent-") as temporary:
            temporary = Path(temporary)
            exported = temporary / "export"
            exported.mkdir()
            export_sources(source, exported)
            if any(exported.rglob("go.work")):
                raise ValueError("workspace file entered exported checkout")
            environment = {**os.environ, "GOWORK": "off", "GOFLAGS": "-mod=readonly", "GOCACHE": str(temporary / "go-build")}
            if not arguments.online:
                environment["GOPROXY"] = "off"
            for index, module in enumerate(selected):
                location = exported / module["directory"]
                print("Independent module: " + module["path"], flush=True)
                run(["go", "build", "./..."], location, environment)
                run(["go", "test", "-count=1", "./..."], location, environment)
                packages = public_packages(location, environment)
                consumer(temporary, index, module, exported, core, environment, packages)
            if arguments.module in (None, "integration"):
                # Private test-only module is independently buildable/testable but is not a consumer release.
                run(["go", "build", "./..."], exported / "integration", environment)
                run(["go", "test", "-count=1", "./..."], exported / "integration", environment)
            if arguments.module in (None, "."):
                check_logger_graph(temporary, exported, core, environment)
            print(f"PASS: {len(selected)} releasable module(s) independently built, tested and imported; GOWORK=off", flush=True)
        return 0
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        print("FAIL: independent module check: " + str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
