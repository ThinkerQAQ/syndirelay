#!/usr/bin/env python3
"""Wrap an extracted release with native Qt IFW; never package runtime data."""
import argparse
import datetime
import pathlib
import platform
import json
import plistlib
import re
import shutil
import subprocess
import tempfile
import xml.etree.ElementTree as ET

ASSETS = pathlib.Path(__file__).with_name("installer")
QA_PREPARE_ONLY = False

PAYLOAD = ("bin", "extension", "renderers", "node_modules", "scripts",
           "package.json", "package-lock.json", "LICENSE", "README.md", "README_ZH.md",
           "THIRD_PARTY_NOTICES.md", "THIRD_PARTY_LICENSES", "Install-SyndiRelay.ps1", "Open-SyndiRelay.ps1",
           "Open-SyndiRelay.sh")

def xml(path, tag, values):
    node = ET.Element(tag)
    for key, value in values.items():
        ET.SubElement(node, key).text = value
    ET.indent(node)
    ET.ElementTree(node).write(path, encoding="utf-8", xml_declaration=True)

def prepare(stage, destination, version, target):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("version must be major.minor.patch")
    suffix = ".exe" if target == "windows" else ""
    for name in ("bin/syndirelay"+suffix, "package.json", "LICENSE", "Open-SyndiRelay.ps1" if target=="windows" else "Open-SyndiRelay.sh"):
        if not (stage/name).is_file():
            raise ValueError("release is missing "+name)
    if not (stage/"THIRD_PARTY_LICENSES"/"MANIFEST.json").is_file() and not (stage/".qa-only").exists():
        raise ValueError("reviewed third-party licenses are required")
    if (stage/".qa-only").exists() and not QA_PREPARE_ONLY:
        raise ValueError("QA-only stage cannot produce a distributable installer")
    # Explicit allowlist. Even accidentally staged secrets must never enter an installer.
    for path in stage.rglob("*"):
        if path.is_symlink():
            raise ValueError("symlink in release stage")
        if path.is_file() and (path.name.lower() in ("config.toml","api-token","extension-access.key","runtime.env","runtime-key.dpapi", "blogctl.toml", "syndirelay.toml", ".env")
                              or path.suffix.lower() in (".sqlite",".db",".log",".har")
                              or any(part.lower() in ("data","backups") for part in path.relative_to(stage).parts)):
            raise ValueError("private runtime file in release stage")
    config = destination/"config"
    package = destination/"packages/com.thinkerqaq.syndirelay"
    meta = package/"meta"
    data = package/"data"
    config.mkdir(parents=True)
    meta.mkdir(parents=True)
    data.mkdir()
    for name in PAYLOAD:
        src = stage/name
        if name == "THIRD_PARTY_LICENSES" and QA_PREPARE_ONLY and not src.exists():
            continue
        if src.is_dir():
            shutil.copytree(src, data/name)
        elif src.is_file():
            shutil.copy2(src, data/name)
    xml(config/"config.xml", "Installer", {
        "Name":"SyndiRelay", "Version":version, "Title":"SyndiRelay",
        "Publisher":"ThinkerQAQ", "StartMenuDir":"SyndiRelay",
        "TargetDir":"@HomeDir@/SyndiRelay", "MaintenanceToolName":"SyndiRelay-Maintenance",
        "RemoveTargetDir":"false", "WizardStyle":"Modern",
        "WizardShowPageList":"false", "WizardDefaultWidth":"680", "WizardDefaultHeight":"520",
        "TitleColor":"#245F73", "ControlScript":"controller.qs", "StyleSheet":"style.qss",
        "Logo":"syndirelay-logo.png", "InstallerWindowIcon":"syndirelay-window",
        "ProductUrl":"https://github.com/ThinkerQAQ/syndirelay",
    })
    if target == "windows":
        import struct
        png = (stage/"extension/icons/icon128.png").read_bytes()
        # Windows .ico supports a PNG payload; no external graphics dependency.
        ico = struct.pack("<HHH", 0, 1, 1) + struct.pack("<BBBBHHII", 128, 128, 0, 0, 1, 32, len(png), 22) + png
        (config/"syndirelay-application.ico").write_bytes(ico)
        tree = ET.parse(config/"config.xml")
        ET.SubElement(tree.getroot(), "InstallerApplicationIcon").text = "syndirelay-application"
        ET.indent(tree)
        tree.write(config/"config.xml", encoding="utf-8", xml_declaration=True)
    for name in ("controller.qs", "style.qss"):
        shutil.copy2(ASSETS/name, config/name)
    logo = stage/"extension/icons/icon128.png"
    if not logo.is_file():
        raise ValueError("release is missing extension/icons/icon128.png")
    # IFW requires distinct filenames for image roles.
    shutil.copy2(logo, config/"syndirelay-logo.png")
    shutil.copy2(logo, config/"syndirelay-window.png")
    shutil.copy2(ASSETS/"first-run.ui", meta/"first-run.ui")
    shutil.copy2(stage/"LICENSE", meta/"license.txt")
    if target == "windows":
        (data/"scripts").mkdir(exist_ok=True)
        shutil.copy2(ASSETS/"first-run.ps1", data/"scripts/SyndiRelay-FirstRun.ps1")
    xml(meta/"package.xml", "Package", {
        "DisplayName":"SyndiRelay", "Description":"Local article syndication and publishing",
        "Version":version, "ReleaseDate":datetime.date.today().isoformat(),
        "Default":"true", "Script":"installscript.qs",
    })
    package_xml = meta/"package.xml"
    package_tree = ET.parse(package_xml)
    ET.SubElement(ET.SubElement(package_tree.getroot(), "UserInterfaces"), "UserInterface").text = "first-run.ui"
    ET.SubElement(ET.SubElement(package_tree.getroot(), "Licenses"), "License",
                  {"name":"MIT (SyndiRelay original code)", "file":"license.txt"})
    ET.indent(package_tree)
    package_tree.write(package_xml, encoding="utf-8", xml_declaration=True)
    script = """function Component() {
    if (installer.isInstaller() && !installer.isCommandLineInstance()) {
        installer.installationFinished.connect(this, Component.prototype.showFirstRunOptions);
        installer.finishButtonClicked.connect(this, Component.prototype.finishFirstRun);
    }
}
Component.prototype.createOperations = function() {
    component.createOperations();
"""
    if target == "windows":
        script += '    component.addOperation("Mkdir", "@UserStartMenuProgramsPath@/SyndiRelay");\n'
        script += '    component.addOperation("CreateShortcut", installer.environmentVariable("SystemRoot") + "/System32/WindowsPowerShell/v1.0/powershell.exe", "@UserStartMenuProgramsPath@/SyndiRelay/SyndiRelay.lnk", ' + json.dumps('-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "@TargetDir@/Open-SyndiRelay.ps1"') + ', "workingDirectory=@TargetDir@");\n'
        script += '    component.addOperation("CreateShortcut", "@TargetDir@/SyndiRelay-Maintenance.exe", "@UserStartMenuProgramsPath@/SyndiRelay/Uninstall SyndiRelay.lnk");\n'
    elif target == "linux":
        entry = 'Type=Application\nName=SyndiRelay\nExec=/bin/bash "@TargetDir@/Open-SyndiRelay.sh"\nTerminal=false\nCategories=Utility;'
        script += '    component.addOperation("CreateDesktopEntry", "syndirelay.desktop", ' + json.dumps(entry) + ');\n'
    else:
        app = data/"SyndiRelay.app/Contents"
        (app/"MacOS").mkdir(parents=True)
        (app/"Info.plist").write_bytes(plistlib.dumps({"CFBundleName":"SyndiRelay", "CFBundleIdentifier":"com.thinkerqaq.syndirelay", "CFBundleExecutable":"SyndiRelay", "CFBundleVersion":version, "CFBundlePackageType":"APPL"}))
        launcher = app/"MacOS/SyndiRelay"
        launcher.write_text('#!/bin/bash\nset -euo pipefail\nroot="$(cd "$(dirname "$0")/../../.." && pwd)"\nexec /bin/bash "$root/Open-SyndiRelay.sh"\n')
        launcher.chmod(0o755)
    script += "};\n"
    script += (ASSETS/"finish.qs").read_text(encoding="utf-8").replace("__PLATFORM__", target)
    (meta/"installscript.qs").write_text(script, encoding="utf-8")

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stage", type=pathlib.Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--os", choices=("windows","linux","darwin"), required=True)
    parser.add_argument("--binarycreator", default="binarycreator")
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--prepare-only", action="store_true", help="Generate inspectable IFW tree instead of an installer")
    args=parser.parse_args()
    global QA_PREPARE_ONLY
    QA_PREPARE_ONLY = args.prepare_only
    if args.output.exists():
        parser.error("refusing to overwrite output")
    if args.prepare_only:
        prepare(args.stage.resolve(),args.output.resolve(),args.version,args.os)
        return
    host={"Windows":"windows","Linux":"linux","Darwin":"darwin"}[platform.system()]
    if host!=args.os:
        parser.error("Qt IFW installers must be built on the matching native OS")
    args.output.parent.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="syndirelay-ifw-") as temp:
        tree=pathlib.Path(temp)
        prepare(args.stage.resolve(),tree,args.version,args.os)
        subprocess.run([args.binarycreator,"--offline-only","-c",str(tree/"config/config.xml"),
                        "-p",str(tree/"packages"),str(args.output.resolve())],check=True)
if __name__=="__main__":
    main()
