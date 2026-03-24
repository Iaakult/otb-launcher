#!/usr/bin/env python3

import argparse
import hashlib
import json
import lzma
import shutil
from copy import deepcopy
from datetime import datetime
from pathlib import Path
from urllib.parse import quote


DEFAULT_SITE_ROOT = Path("/var/www/html/launcher")
DEFAULT_TIBIA_SRC = Path("/home/iaakult/Downloads/OTBaiak Client")
DEFAULT_OTCLIENT_SRC = Path("/home/iaakult/Downloads/OTBaiak - OTC")
DEFAULT_LAUNCHER_EXE = Path("/home/iaakult/otb-launcher/build/bin/OTBaiak-Launcher.exe")

GENERIC_IGNORE_NAMES = {
    ".git",
    ".gitignore",
    "otbaiakv2.log",
    "otclient.log",
    "packet.log",
}


def encode_url_path(path: str) -> str:
    return "/".join(quote(part, safe="") for part in path.split("/"))


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def write_json(path: Path, payload: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2) + "\n")


def read_json(path: Path) -> dict:
    return json.loads(path.read_text())


def compress_lzma_alone(src: Path, dst: Path) -> None:
    compressor = lzma.LZMACompressor(format=lzma.FORMAT_ALONE)
    with src.open("rb") as fin, dst.open("wb") as fout:
        for chunk in iter(lambda: fin.read(1024 * 1024), b""):
            fout.write(compressor.compress(chunk))
        fout.write(compressor.flush())


def payload_signature(manifest: dict, ignore_keys: set[str]) -> str:
    payload = {key: value for key, value in manifest.items() if key not in ignore_keys}
    return json.dumps(payload, sort_keys=True, separators=(",", ":"))


def carry_client_revision(old_manifest: dict | None, new_manifest: dict) -> int:
    if not old_manifest:
        return max(1, int(new_manifest.get("revision", 1) or 1))

    old_revision = int(old_manifest.get("revision", 0) or 0)
    ignored_keys = {"revision", "version"}
    if payload_signature(old_manifest, ignored_keys) == payload_signature(new_manifest, ignored_keys):
        return max(1, old_revision)
    return max(1, old_revision + 1)


def carry_assets_version(old_manifest: dict | None, new_manifest: dict) -> int:
    if not old_manifest:
        return max(1, int(new_manifest.get("version", 1) or 1))

    old_version = int(old_manifest.get("version", 0) or 0)
    if payload_signature(old_manifest, {"version"}) == payload_signature(new_manifest, {"version"}):
        return max(1, old_version)
    return max(1, old_version + 1)


def carry_client_version(old_manifest: dict | None, new_manifest: dict, fallback: str) -> str:
    if not old_manifest:
        return fallback

    if payload_signature(old_manifest, {"revision", "version"}) == payload_signature(new_manifest, {"revision", "version"}):
        return str(old_manifest.get("version", fallback))
    return fallback


def copy_launcher(launcher_exe: Path, site_root: Path) -> list[str]:
    if not launcher_exe.exists():
        raise FileNotFoundError(f"Launcher executable not found: {launcher_exe}")

    published = []
    for target_name in ("OTBaiak-Launcher.exe", "OTBaiak.exe"):
        dst = site_root / target_name
        dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(launcher_exe, dst)
        checksum = sha256_file(dst)
        (site_root / f"{target_name}.sha256").write_text(f"{checksum}  {target_name}\n")
        published.append(str(dst))

    return published


def resolve_launcher_exe(path: Path, site_root: Path) -> Path:
    candidates = [
        path,
        path.with_name("OTBaiak.exe"),
        path.with_name("OTBaiak-Launcher"),
        path.with_name("OTBaiak"),
        Path("/home/iaakult/client-repos/OTBaiak-Launcher/build/bin/OTBaiak-Launcher.exe"),
        Path("/home/iaakult/client-repos/OTBaiak-Launcher/build/bin/OTBaiak.exe"),
        Path("/home/iaakult/otb-launcher/build/bin/OTBaiak-Launcher.exe"),
        Path("/home/iaakult/otb-launcher/build/bin/OTBaiak.exe"),
        site_root / "OTBaiak-Launcher.exe",
        site_root / "OTBaiak.exe",
    ]

    for candidate in candidates:
        if candidate.exists() and candidate.is_file():
            return candidate

    searched = "\n - ".join(str(candidate) for candidate in candidates)
    raise FileNotFoundError(
        "Launcher executable not found. Checked:\n - "
        + searched
        + "\nUse --launcher-exe /caminho/para/arquivo.exe"
    )


def materialize_manifest_entry(src_root: Path, dst_root: Path, file_entry: dict) -> tuple[dict | None, dict | None]:
    entry = deepcopy(file_entry)
    url = entry["url"]
    localfile = entry["localfile"]
    dst = dst_root / url
    src_url = src_root / url
    src_local = src_root / localfile

    if not src_url.exists() and not src_local.exists():
        return None, {"url": url, "localfile": localfile}

    dst.parent.mkdir(parents=True, exist_ok=True)
    if src_url.exists():
        shutil.copy2(src_url, dst)
    elif url.endswith(".lzma"):
        compress_lzma_alone(src_local, dst)
    else:
        shutil.copy2(src_local, dst)

    if src_local.exists():
        entry["unpackedhash"] = sha256_file(src_local)
        entry["unpackedsize"] = src_local.stat().st_size
    entry["url"] = encode_url_path(entry["url"])
    entry["packedhash"] = sha256_file(dst)
    entry["packedsize"] = dst.stat().st_size
    return entry, None


def publish_tibia_windows(src_root: Path, site_root: Path) -> dict:
    client_manifest = read_json(src_root / "package.json")
    assets_manifest = read_json(src_root / "assets.json")

    dst_root = site_root / "tibia1511"
    old_client_manifest_path = dst_root / "client.windows.json"
    old_assets_manifest_path = dst_root / "assets.windows.json"
    old_client_manifest = read_json(old_client_manifest_path) if old_client_manifest_path.exists() else None
    old_assets_manifest = read_json(old_assets_manifest_path) if old_assets_manifest_path.exists() else None
    if dst_root.exists():
        shutil.rmtree(dst_root)
    dst_root.mkdir(parents=True, exist_ok=True)

    client_files = []
    skipped_client = []
    for file_entry in client_manifest.get("files", []):
        published, skipped = materialize_manifest_entry(src_root, dst_root, file_entry)
        if published:
            client_files.append(published)
        if skipped:
            skipped_client.append(skipped)

    assets_files = []
    skipped_assets = []
    for file_entry in assets_manifest.get("files", []):
        published, skipped = materialize_manifest_entry(src_root, dst_root, file_entry)
        if published:
            assets_files.append(published)
        if skipped:
            skipped_assets.append(skipped)

    client_manifest["files"] = client_files
    client_manifest["variant"] = "windows"
    client_manifest["executable"] = "bin/client.exe"
    client_manifest["revision"] = carry_client_revision(old_client_manifest, client_manifest)

    assets_manifest["files"] = assets_files
    assets_manifest["version"] = carry_assets_version(old_assets_manifest, assets_manifest)

    write_json(dst_root / "client.windows.json", client_manifest)
    write_json(dst_root / "assets.windows.json", assets_manifest)
    write_json(dst_root / "publish-skipped.json", {"client": skipped_client, "assets": skipped_assets})

    return {
        "client_files": len(client_files),
        "asset_files": len(assets_files),
        "skipped_client_files": len(skipped_client),
        "skipped_asset_files": len(skipped_assets),
        "revision": client_manifest["revision"],
        "version": client_manifest.get("version"),
    }


def iter_generic_files(src_root: Path):
    for path in sorted(src_root.rglob("*")):
        if not path.is_file():
            continue
        relative = path.relative_to(src_root)
        if any(part in GENERIC_IGNORE_NAMES for part in relative.parts):
            continue
        if path.name in GENERIC_IGNORE_NAMES:
            continue
        yield path


def publish_generic_windows_client(src_root: Path, site_root: Path, game_id: str, executable_name: str) -> dict:
    dst_root = site_root / game_id
    old_client_manifest_path = dst_root / "client.windows.json"
    old_assets_manifest_path = dst_root / "assets.windows.json"
    old_client_manifest = read_json(old_client_manifest_path) if old_client_manifest_path.exists() else None
    old_assets_manifest = read_json(old_assets_manifest_path) if old_assets_manifest_path.exists() else None

    if dst_root.exists():
        shutil.rmtree(dst_root)
    dst_root.mkdir(parents=True, exist_ok=True)

    files = []
    for path in iter_generic_files(src_root):
        relative = path.relative_to(src_root).as_posix()
        dst = dst_root / relative
        dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(path, dst)
        checksum = sha256_file(path)
        size = path.stat().st_size
        files.append(
            {
                "url": encode_url_path(relative),
                "localfile": relative,
                "packedhash": checksum,
                "packedsize": size,
                "unpackedhash": checksum,
                "unpackedsize": size,
            }
        )

    version_fallback = datetime.now().strftime("%Y.%m.%d.%H%M")
    client_manifest = {
        "revision": 1,
        "version": version_fallback,
        "files": files,
        "executable": executable_name,
        "generation": "custom",
        "variant": "windows",
    }
    client_manifest["revision"] = carry_client_revision(old_client_manifest, client_manifest)
    client_manifest["version"] = carry_client_version(old_client_manifest, client_manifest, version_fallback)

    assets_manifest = {"version": 1, "files": []}
    assets_manifest["version"] = carry_assets_version(old_assets_manifest, assets_manifest)

    if old_client_manifest and old_client_manifest_path.exists():
        legacy_linux = dst_root / "client.linux.json"
        if legacy_linux.exists():
            legacy_linux.unlink()
    write_json(dst_root / "client.windows.json", client_manifest)
    write_json(dst_root / "assets.windows.json", assets_manifest)

    for stale_name in ("client.linux.json", "assets.linux.json"):
        stale_path = dst_root / stale_name
        if stale_path.exists():
            stale_path.unlink()

    return {
        "client_files": len(files),
        "asset_files": 0,
        "revision": client_manifest["revision"],
        "version": client_manifest["version"],
    }


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Publish OTBaiak launcher and client assets to the website repo.")
    parser.add_argument("--site-root", type=Path, default=DEFAULT_SITE_ROOT)
    parser.add_argument("--tibia-src", type=Path, default=DEFAULT_TIBIA_SRC)
    parser.add_argument("--otclient-src", type=Path, default=DEFAULT_OTCLIENT_SRC)
    parser.add_argument("--launcher-exe", type=Path, default=DEFAULT_LAUNCHER_EXE)
    parser.add_argument("--skip-launcher", action="store_true")
    parser.add_argument("--skip-tibia", action="store_true")
    parser.add_argument("--skip-otclient", action="store_true")
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    summary = {}
    args.site_root.mkdir(parents=True, exist_ok=True)

    if not args.skip_launcher:
        launcher_exe = resolve_launcher_exe(args.launcher_exe, args.site_root)
        published = copy_launcher(launcher_exe, args.site_root)
        summary["launcher"] = {"published": published}

    if not args.skip_tibia:
        summary["tibia1511"] = publish_tibia_windows(args.tibia_src, args.site_root)

    if not args.skip_otclient:
        summary["otclient"] = publish_generic_windows_client(
            args.otclient_src,
            args.site_root,
            "otclient",
            "OTBaiak OTC.exe",
        )

    print(json.dumps(summary, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())