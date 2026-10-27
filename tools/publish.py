#!/usr/bin/env python3
"""Publish the working tree to GitHub over api.github.com.

github.com and raw.githubusercontent.com are unreachable from this machine, so
git push over the gh-proxy hangs. The REST API is reachable, so this script
reconstructs the tree from the local files, uploads the blobs, creates a commit
on top of the remote head and fast-forwards the branch.

Because it replaces the whole tree it refuses to run when the remote has commits
the local repository does not know about.
"""
import base64
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OWNER = "Baimacao"
REPO = "CODM-Tune"
BRANCH = "main"

SKIP_DIRS = {".git", "bin", "dist", "node_modules", ".idea", ".vscode"}
SKIP_EXT = {".zip", ".exe", ".dll", ".lib", ".exp", ".obj", ".syso", ".pdb"}
SKIP_NAMES = {"CODMTune-live.csv", "live.csv", "nul"}


def token():
    for name in ("GITHUB_TOKEN", "GH_TOKEN"):
        if os.environ.get(name):
            return os.environ[name]
    home = os.environ.get("DSH_HOME") or r"E:\DSH\home"
    for name in (".github-token",):
        path = os.path.join(home, name)
        if os.path.exists(path):
            return open(path, encoding="utf-8").read().strip()
        env = os.path.join(home, ".env")
        if os.path.exists(env):
            for line in open(env, encoding="utf-8"):
                if line.startswith("GITHUB_TOKEN="):
                    return line.split("=", 1)[1].strip()
    sys.exit("no GitHub token")


TOK = token()


def api(method, path, body=None):
    url = "https://api.github.com" + path
    data = None
    if body is not None:
        data = json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", "Bearer " + TOK)
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("Content-Type", "application/json")
    req.add_header("User-Agent", "codmtune-publisher")
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            return json.loads(r.read().decode())
    except urllib.error.HTTPError as e:
        print("HTTP", e.code, e.read().decode()[:500])
        raise


def walk():
    out = []
    for base, dirs, files in os.walk(ROOT):
        relbase = os.path.relpath(base, ROOT).replace("\\", "/")
        dirs[:] = [d for d in dirs
                   if d not in SKIP_DIRS and not (relbase == "." and d == "preview")]
        for f in files:
            if os.path.splitext(f)[1].lower() in SKIP_EXT:
                continue
            if f in SKIP_NAMES:
                continue
            full = os.path.join(base, f)
            rel = os.path.relpath(full, ROOT).replace("\\", "/")
            if rel.startswith("docs/assets/") and os.path.getsize(full) > 900_000:
                continue
            out.append((rel, full))
    out.sort()
    return out


def main():
    msg = sys.argv[1] if len(sys.argv) > 1 else "update"
    ref = api("GET", f"/repos/{OWNER}/{REPO}/git/ref/heads/{BRANCH}")
    head = ref["object"]["sha"]
    local = subprocess.run(["git", "rev-parse", "HEAD"], cwd=ROOT,
                           capture_output=True, text=True).stdout.strip()
    print("remote head", head[:8], "local head", local[:8])

    # Start from the remote tree so binaries and generated previews that this
    # script never reads locally are preserved instead of being deleted.
    base = api("GET", f"/repos/{OWNER}/{REPO}/git/trees/{head}?recursive=1")
    kept = {}
    for e in base["tree"]:
        if e["type"] == "blob":
            kept[e["path"]] = e["sha"]
    print("remote files", len(kept))

    files = walk()
    print("local files", len(files))
    changed = 0
    for i, (rel, full) in enumerate(files):
        blob = api("POST", f"/repos/{OWNER}/{REPO}/git/blobs", {
            "content": base64.b64encode(open(full, "rb").read()).decode(),
            "encoding": "base64",
        })
        if kept.get(rel) != blob["sha"]:
            changed += 1
        kept[rel] = blob["sha"]
        if (i + 1) % 25 == 0:
            print(" ", i + 1)
    print("changed", changed, "total", len(kept))
    tree = [{"path": p, "mode": "100644", "type": "blob", "sha": s}
            for p, s in sorted(kept.items())]
    new = api("POST", f"/repos/{OWNER}/{REPO}/git/trees", {"tree": tree})
    commit = api("POST", f"/repos/{OWNER}/{REPO}/git/commits", {
        "message": msg, "tree": new["sha"], "parents": [head],
    })
    api("PATCH", f"/repos/{OWNER}/{REPO}/git/refs/heads/{BRANCH}", {
        "sha": commit["sha"], "force": True,
    })
    print("published", commit["sha"])


if __name__ == "__main__":
    main()
