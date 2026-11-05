"""Bundle pinned official OpenAI Tunnel Client and its cloudflared sidecar.

This is strictly a build-time downloader; the resulting installer does not need
network access to install or start the tunnel client. Both upstream assets are
verified against the published release's SHA-256 digests.
"""
import hashlib
import io
import os
from pathlib import Path
import urllib.request
import zipfile

TUNNEL_TAG = "v0.0.19"
SOURCE_BASE = f"https://github.com/openai/tunnel-client/releases/download/{TUNNEL_TAG}"
# Official 2026-10-30 release checksums, not locally generated hashes.
TUNNEL_ARCHIVES = {
    ("windows", "amd64"): "1d0ecc3a54f2a51e27a635ac155f40fbb95df58c52cdeddf6c75532cc0bbeb25",
    ("windows", "arm64"): "4f0ad0501bf260d39369e6879661b5adf58b1d5ff01e4668605e0a32738d4445",
    ("linux", "amd64"): "9ee9448592345a9dd1517fb79791ab54d24f051142eb92bcbe8cd355effc9142",
    ("linux", "arm64"): "5a19e556c4a8736ab7a6fde6f0568f6e7411fa2dff21186fc15bc30e9b98a603",
    ("darwin", "amd64"): "dff97674c77cac59e6ab3d901e6cff50fb17a5784c01d8959d1f4adb3afdf4bf",
    ("darwin", "arm64"): "2243d024b77af8ee4b4ab7a1447949e5fc8d82bcd2d51b178927f2da9e47ecfd",
}
TUNNEL_LICENSES = {
    "windows": "2b1def83c55dea817579ea1a7206119356c6a97a190c31181fe7817ae60c29ec",
    "linux": "338065faea9beccb5d218b4141a7876e3f18bee34a0844ee33001f1229bc6650",
    "darwin": "4f497f45e64a0ce7c06e3f7c90260fe4ad941bbb1a56c6bbad6f8797f62b2dfb",
}

def fetch_verified(url, expected_digest, max_bytes):
    request = urllib.request.Request(url, headers={"User-Agent": "SyndiRelay-Release-Builder"})
    with urllib.request.urlopen(request, timeout=120) as response:
        data = response.read(max_bytes + 1)
    if len(data) > max_bytes:
        raise ValueError("official tunnel asset exceeds size limit")
    if hashlib.sha256(data).hexdigest() != expected_digest:
        raise ValueError("official Tunnel Client SHA-256 digest mismatch")
    return data

def bundle_tunnel_client(stage, goos, goarch):
    key = goos, goarch
    if key not in TUNNEL_ARCHIVES:
        raise ValueError(f"unsupported Tunnel Client platform {goos}-{goarch}")
    platform = f"{goos}-{goarch}"
    prefix = f"tunnel-client-{TUNNEL_TAG}-{platform}"
    archive = fetch_verified(f"{SOURCE_BASE}/{prefix}.zip", TUNNEL_ARCHIVES[key], 120 << 20)
    license_bytes = fetch_verified(f"{SOURCE_BASE}/{prefix}-licenses.txt",
                                   TUNNEL_LICENSES[goos], 1 << 20)
    ext = ".exe" if goos == "windows" else ""
    expected_files = [f"tunnel-client{ext}", f"cloudflared{ext}"]
    bin_dir = Path(stage) / "bin"
    with zipfile.ZipFile(io.BytesIO(archive)) as package:
        for name in expected_files:
            try:
                info = package.getinfo(name)
            except KeyError as exc:
                raise ValueError(f"verified Tunnel Client archive missing {name}") from exc
            if info.is_dir() or info.file_size <= 0 or info.file_size > (80 << 20):
                raise ValueError(f"invalid Tunnel Client binary: {name}")
            if (info.external_attr >> 16) & 0o170000 == 0o120000:
                raise ValueError(f"symlink in verified Tunnel Client archive: {name}")
            target = bin_dir / name
            if target.exists():
                raise ValueError(f"Tunnel Client binary already exists: {target}")
            target.write_bytes(package.read(name))
            if goos != "windows":
                target.chmod(0o755)
    license_dir = Path(stage) / "THIRD_PARTY_LICENSES" / "runtime" / "tunnel-client"
    license_dir.mkdir(parents=True, exist_ok=True)
    (license_dir / "LICENSES.txt").write_bytes(license_bytes)
    (license_dir / "VERSION.txt").write_text(TUNNEL_TAG + "\n", encoding="utf-8")
