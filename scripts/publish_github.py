#!/usr/bin/env python3
"""Publish this source tree and its release artifacts to GitHub."""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
from pathlib import Path
from urllib.parse import urlparse


VERSION = "0.1.0-rc.1"
DEFAULT_REPOSITORY = "wiibridge"
DESCRIPTION = "TrueNAS WBFS host and Raspberry Pi NBD-to-USB bridge"
RELEASE_NOTE = "SOFTWARE-COMPLETE RELEASE CANDIDATE — HARDWARE UNVERIFIED"
GIT_MAX_BYTES = 100 * 1024 * 1024
RELEASE_MAX_BYTES = 2 * 1024 * 1024 * 1024

IGNORE_RULES = """\
# Generated build state and private test material.
/build/
/testdata/private/*
!/testdata/private/.gitkeep

# Binary releases are uploaded as GitHub Release assets.
/dist/*.img
/dist/*.img.xz
/dist/*.oci/
/dist/*.bmap

# Deployment secrets; examples remain tracked.
.env
.env.*
!.env.example
deploy/truenas/.env
"""

RELEASE_PATTERNS = (
    "*.img.xz",
    "*.sha256",
    "*.bmap",
    "*.packages.txt",
    "*.sbom.spdx.json",
    "*.provenance.json",
    "*.offline-validation.json",
    "*.build.log",
    "wiibridge-host-*.digest",
    "wiibridge-host-*.oci.index.sha256",
)


def run(
    command: list[str],
    *,
    cwd: Path,
    capture: bool = False,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    print("+", " ".join(command))
    return subprocess.run(
        command,
        cwd=cwd,
        check=check,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
    )


def output(command: list[str], root: Path) -> str:
    completed = run(command, cwd=root, capture=True, check=False)
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        message = f"command failed ({completed.returncode}): {' '.join(command)}"
        if detail:
            message += f"\n{detail}"
        raise RuntimeError(message)
    return completed.stdout.strip()


def append_ignore_rules(root: Path) -> None:
    ignore_file = root / ".gitignore"
    existing = ignore_file.read_text(encoding="utf-8") if ignore_file.exists() else ""
    missing = [line for line in IGNORE_RULES.splitlines() if line and line not in existing.splitlines()]
    if not missing:
        return
    separator = "" if not existing or existing.endswith("\n") else "\n"
    ignore_file.write_text(
        existing + separator + "\n# Added by scripts/publish_github.py\n" + "\n".join(missing) + "\n",
        encoding="utf-8",
    )


def require_tools() -> None:
    missing = [tool for tool in ("git", "gh") if shutil.which(tool) is None]
    if missing:
        raise RuntimeError("missing required command(s): " + ", ".join(missing))


def ensure_repository(root: Path) -> None:
    if not (root / ".git").exists():
        run(["git", "init", "-b", "main"], cwd=root)
    private_fixture = root / "testdata/private"
    if private_fixture.exists():
        unexpected = [
            item for item in private_fixture.rglob("*")
            if item.is_file() and item.name != ".gitkeep"
        ]
        if unexpected:
            print(
                f"Private fixtures found and excluded from Git: {len(unexpected)} file(s).",
                file=sys.stderr,
            )


def check_staged_files(root: Path) -> None:
    names = output(["git", "diff", "--cached", "--name-only", "-z"], root).split("\0")
    violations: list[str] = []
    for name in filter(None, names):
        path = root / name
        if not path.is_file() or path.is_symlink():
            continue
        if path.stat().st_size > GIT_MAX_BYTES:
            violations.append(f"{name} ({path.stat().st_size} bytes)")
        lowered = name.lower()
        if lowered.startswith("testdata/private/") and path.name != ".gitkeep":
            violations.append(f"{name} (private test fixture)")
        if path.name in {"server.key", "device.key", "id_rsa", "id_ed25519"}:
            violations.append(f"{name} (probable private key)")
    if violations:
        raise RuntimeError("refusing to commit unsafe/oversized files:\n  " + "\n  ".join(violations))


def commit_changes(root: Path, message: str) -> None:
    run(["git", "add", "-A"], cwd=root)
    check_staged_files(root)
    staged = run(["git", "diff", "--cached", "--quiet"], cwd=root, check=False)
    if staged.returncode == 0:
        print("No changes to commit.")
        return
    run(["git", "commit", "-m", message], cwd=root)


def ensure_authentication(root: Path) -> None:
    status = run(["gh", "auth", "status"], cwd=root, check=False)
    if status.returncode != 0:
        print("GitHub authentication is required; starting interactive login.")
        run(["gh", "auth", "login"], cwd=root)


def remote_exists(root: Path, remote: str) -> bool:
    return run(["git", "remote", "get-url", remote], cwd=root, capture=True, check=False).returncode == 0


def repository_from_remote(root: Path, remote: str) -> str:
    """Return the OWNER/REPO identifier encoded in a GitHub remote URL."""
    remote_url = output(["git", "remote", "get-url", remote], root)
    if "://" in remote_url:
        parsed = urlparse(remote_url)
        host = parsed.hostname
        path = parsed.path
    else:
        # Git's SCP-like SSH syntax: git@github.com:OWNER/REPO.git
        host_path = remote_url.split(":", 1)
        if len(host_path) != 2:
            raise RuntimeError(f"cannot determine GitHub repository from {remote} remote: {remote_url}")
        host = host_path[0].rsplit("@", 1)[-1]
        path = host_path[1]
    repository = path.strip("/")
    if repository.endswith(".git"):
        repository = repository[:-4]
    if not host or repository.count("/") != 1:
        raise RuntimeError(f"cannot determine GitHub repository from {remote} remote: {remote_url}")
    return repository if host == "github.com" else f"{host}/{repository}"


def create_or_push_repository(
    root: Path,
    repository: str,
    visibility: str,
    remote: str,
) -> None:
    if remote_exists(root, remote):
        print(f"Using existing {remote} remote: {output(['git', 'remote', 'get-url', remote], root)}")
        run(["git", "push", "-u", remote, "HEAD"], cwd=root)
        return
    command = [
        "gh", "repo", "create", repository,
        f"--{visibility}",
        "--description", DESCRIPTION,
        "--source=.",
        f"--remote={remote}",
        "--push",
    ]
    run(command, cwd=root)


def release_assets(root: Path) -> list[Path]:
    dist = root / "dist"
    assets: dict[Path, None] = {}
    for pattern in RELEASE_PATTERNS:
        for path in dist.glob(pattern):
            if path.is_file():
                if path.stat().st_size > RELEASE_MAX_BYTES:
                    raise RuntimeError(f"release asset exceeds 2 GiB: {path}")
                assets[path] = None
    return sorted(assets)


def publish_release(root: Path, tag: str, owner_repo: str) -> None:
    assets = release_assets(root)
    if not assets:
        raise RuntimeError("no release assets found under dist/")
    existing = run(
        ["gh", "release", "view", tag, "--repo", owner_repo],
        cwd=root,
        capture=True,
        check=False,
    )
    asset_names = [str(path.relative_to(root)) for path in assets]
    if existing.returncode == 0:
        run(
            ["gh", "release", "upload", tag, "--repo", owner_repo, "--clobber", *asset_names],
            cwd=root,
        )
    else:
        run(
            [
                "gh", "release", "create", tag,
                "--repo", owner_repo,
                "--prerelease",
                "--title", f"WiiBridge {VERSION}",
                "--notes", RELEASE_NOTE,
                *asset_names,
            ],
            cwd=root,
        )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=DEFAULT_REPOSITORY, help="GitHub repository name")
    visibility = parser.add_mutually_exclusive_group()
    visibility.add_argument("--private", action="store_const", const="private", dest="visibility")
    visibility.add_argument(
        "--public",
        action="store_const",
        const="public",
        dest="visibility",
        help="explicitly create a public repository",
    )
    parser.set_defaults(visibility="private")
    parser.add_argument("--remote", default="origin")
    parser.add_argument("--tag", default=f"v{VERSION}")
    parser.add_argument("--message", default=f"Build WiiBridge {VERSION}")
    parser.add_argument("--no-release", action="store_true", help="push source without a GitHub release")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    root = Path(__file__).resolve().parent.parent
    try:
        require_tools()
        ensure_repository(root)
        append_ignore_rules(root)
        commit_changes(root, args.message)
        ensure_authentication(root)
        create_or_push_repository(root, args.repo, args.visibility, args.remote)
        owner_repo = repository_from_remote(root, args.remote)
        if not args.no_release:
            publish_release(root, args.tag, owner_repo)
        url = output(
            ["gh", "repo", "view", owner_repo, "--json", "url", "--jq", ".url"],
            root,
        )
        print(f"\nPublished successfully: {url}")
        return 0
    except (RuntimeError, subprocess.CalledProcessError) as error:
        print(f"\nERROR: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
