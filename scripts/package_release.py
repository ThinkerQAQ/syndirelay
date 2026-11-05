#!/usr/bin/env python3
"""Create a native portable SyndiRelay stage; no user state, credentials or checkout."""
import argparse
import hashlib
import json
import os
import pathlib
import platform
import shutil
import subprocess
import tempfile
import zipfile
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
OS_NAME = {"Windows": "windows", "Darwin": "darwin", "Linux": "linux"}
ARCH_NAME = {"x86_64": "amd64", "AMD64": "amd64", "aarch64": "arm64", "arm64": "arm64"}
ENTRY_SCRIPTS = ("Install-SyndiRelay.ps1", "Open-SyndiRelay.ps1", "Open-SyndiRelay.sh")
SOURCE = ("extension", "renderers", "scripts", "README.md", "README_ZH.md", "LICENSE",
          "THIRD_PARTY_NOTICES.md", "package.json", "package-lock.json")


def build(stage, version, goos, goarch, qa_only=False):
    for name in ("extension/manifest.json", "package.json", "package-lock.json"):
        doc = json.loads((ROOT / name).read_text(encoding="utf-8"))
        if doc.get("version") != version:
            raise ValueError(f"release version {version} does not match {name}")
    if json.loads((ROOT / "package-lock.json").read_text(encoding="utf-8"))["packages"][""]["version"] != version:
        raise ValueError("release and npm lockfile root versions differ")
    if goos not in ("windows", "linux", "darwin") or goarch not in ("amd64", "arm64"):
        raise ValueError("unsupported platform")
    # Public packages use the existing repo-wide reviewed-license gate, including
    # the full Go and npm dependency graphs. QA stages cannot be distributed.
    license_root = ROOT / ".build" / "out" / "publish-licenses"
    if not qa_only:
        subprocess.run(["go", "run", "./cmd/maint", "licenses"], cwd=ROOT, check=True)
    stage.mkdir(parents=True, exist_ok=False)
    for item in SOURCE:
        source = ROOT / item
        if source.is_dir():
            shutil.copytree(source, stage / item, ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
        else:
            shutil.copy2(source, stage / item)
    for filename in ENTRY_SCRIPTS:
        shutil.copy2(ROOT / "scripts" / filename, stage / filename)
    if qa_only:
        (stage / ".qa-only").write_text("Unreviewed build; distribution forbidden.\n")
    else:
        shutil.copytree(license_root, stage / "THIRD_PARTY_LICENSES")
    (stage / "bin").mkdir()
    binary = stage / "bin" / ("syndirelay.exe" if goos == "windows" else "syndirelay")
    env = dict(os.environ, GOOS=goos, GOARCH=goarch, CGO_ENABLED="0")
    subprocess.run(["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(binary), "./cmd"],
                   cwd=ROOT, env=env, check=True)
    if goos == "windows":
        launcher = stage / "bin" / "syndirelay-open.exe"
        subprocess.run(["go","build","-trimpath","-ldflags=-s -w -H=windowsgui",
                        "-o",str(launcher),"./cmd"],cwd=ROOT,env=env,check=True)
    # Dependencies contain the platform-specific Sharp binary. Install only on the matching native host.
    if goos != OS_NAME.get(platform.system()) or goarch != ARCH_NAME.get(platform.machine()):
        raise ValueError("renderer dependencies must be installed on the matching native OS/architecture")
    npm = "npm.cmd" if os.name == "nt" else "npm"
    subprocess.run([npm, "ci", "--omit=dev", "--no-audit", "--no-fund"],
                   cwd=stage, check=True)
    # npm's command shim symlinks must not be included in portable archives/installer.
    binlinks = stage / "node_modules" / ".bin"
    if binlinks.exists():
        shutil.rmtree(binlinks)
    if goos != "windows":
        binary.chmod(0o755)
    for p in [stage / "Open-SyndiRelay.sh"]:
        if p.exists():
            p.chmod(0o755)
    return binary


def archive_stage(stage, output, goos):
    output.parent.mkdir(parents=True, exist_ok=True)
    if output.exists():
        raise ValueError("refusing to overwrite release artifact")
    if goos == "windows":
        with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=8) as pack:
            for p in sorted(stage.rglob("*")):
                if p.is_file():
                    pack.write(p, pathlib.Path("SyndiRelay") / p.relative_to(stage))
    else:
        with tarfile.open(output, "w:gz") as pack:
            pack.add(stage, arcname="SyndiRelay")
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    output.with_name(output.name + ".sha256").write_text(f"{digest}  {output.name}\n", encoding="ascii")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--version", default=json.loads((ROOT/"extension/manifest.json").read_text())["version"])
    ap.add_argument("--os", default=OS_NAME.get(platform.system()))
    ap.add_argument("--arch", default=ARCH_NAME.get(platform.machine()))
    ap.add_argument("--stage", type=pathlib.Path, help="Keep the extracted release payload at this path")
    ap.add_argument("--output", type=pathlib.Path, help="Create the portable archive and SHA-256")
    ap.add_argument("--qa-only", action="store_true", help="Build a non-distributable test stage without the legal review bundle")
    args = ap.parse_args()
    if args.qa_only and args.output:
        ap.error("QA-only stages cannot be distributed")
    if not args.stage and not args.output:
        ap.error("choose --stage and/or --output")
    if args.stage and args.stage.exists():
        ap.error("refusing to overwrite stage")
    if args.stage:
        stage = args.stage.resolve()
        build(stage, args.version, args.os, args.arch, qa_only=args.qa_only)
        if args.output:
            archive_stage(stage, args.output.resolve(), args.os)
    else:
        with tempfile.TemporaryDirectory(prefix="syndirelay-release-") as temp:
            stage = pathlib.Path(temp)/"SyndiRelay"
            build(stage, args.version, args.os, args.arch, qa_only=args.qa_only)
            archive_stage(stage, args.output.resolve(), args.os)
    print("PACKAGED", args.os, args.arch, args.version)


if __name__ == "__main__":
    main()
