#!/usr/bin/env python3

import argparse
import hashlib
import json
import shutil
import zipfile
from pathlib import Path


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


def should_ignore(path: Path, src_root: Path) -> bool:
    relative = path.relative_to(src_root)
    if any(part in GENERIC_IGNORE_NAMES for part in relative.parts):
        return True
    return path.name in GENERIC_IGNORE_NAMES


def iter_files(src_root: Path):
    for path in sorted(src_root.rglob("*")):
        if not path.is_file():
            continue
        if should_ignore(path, src_root):
            continue
        yield path


def create_game_zip(src_root: Path, zip_path: Path) -> dict:
    if not src_root.exists():
        raise FileNotFoundError(f"Source directory not found: {src_root}")

    zip_path.parent.mkdir(parents=True, exist_ok=True)
    file_count = 0
    with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_STORED) as archive:
        for path in iter_files(src_root):
            archive.write(path, arcname=path.relative_to(src_root).as_posix())
            file_count += 1

    return {
        "zip": str(zip_path),
        "files": file_count,
        "size": zip_path.stat().st_size,
        "sha256": sha256_file(zip_path),
    }


def cleanup_legacy_layout(site_root: Path, game_id: str) -> None:
    legacy_dir = site_root / game_id
    if legacy_dir.exists() and legacy_dir.is_dir():
        shutil.rmtree(legacy_dir)


def update_central_version_json(site_root: Path, game_id: str, executable: str) -> None:
    version_path = site_root / "version.json"
    existing: dict = read_json(version_path) if version_path.exists() else {}
    entry = existing.get(game_id, {})
    changed = False
    if not entry:
        entry = {"version": "1.0", "executable": executable}
        changed = True
    elif entry.get("executable") != executable:
        entry["executable"] = executable
        changed = True
    if changed:
        existing[game_id] = entry
        write_json(version_path, existing)
        print(f"  Updated {version_path} [{game_id}]")


def publish_game_zip(src_root: Path, site_root: Path, game_id: str, executable_name: str) -> dict:
    cleanup_legacy_layout(site_root, game_id)
    zip_path = site_root / f"{game_id}.zip"
    summary = create_game_zip(src_root, zip_path)
    update_central_version_json(site_root, game_id, executable_name)
    return summary


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Publish OTBaiak launcher and game zips to the website repo.")
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
        summary["tibia1511"] = publish_game_zip(
            args.tibia_src,
            args.site_root,
            "tibia1511",
            "bin/client.exe",
        )

    if not args.skip_otclient:
        summary["otclient"] = publish_game_zip(
            args.otclient_src,
            args.site_root,
            "otclient",
            "OTBaiak OTC.exe",
        )

    print(json.dumps(summary, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())