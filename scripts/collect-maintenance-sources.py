"""Collect pinned image-package build inputs; never execute APKBUILD or unpack sources.

Optional developer fetch tool. Packaging consumes the resulting verified source
bundle, not this program or a live network connection.
"""
import argparse
import concurrent.futures
import hashlib
import io
import json
import os
import pathlib
import re
import stat
import tarfile
import urllib.parse
import urllib.request
import uuid

ROOT = pathlib.Path(__file__).resolve().parent.parent
LIMIT = 32 << 20
OFFLINE = False


def digest(data, algorithm="sha256"):
    return hashlib.new(algorithm, data).hexdigest()


def read_bounded(path, limit=LIMIT):
    with path.open("rb") as source:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
            raise ValueError("cached source is not a bounded regular file")
        data = source.read(limit + 1)
    if len(data) > limit:
        raise ValueError("cached source exceeds its byte budget")
    return data


def local_path(base, name):
    parts = pathlib.PurePosixPath(name)
    if not name or parts.is_absolute() or any(p in ("", ".", "..") for p in name.split("/")) or "\\" in name or any(ord(c) < 32 for c in name):
        raise ValueError("unsafe source path")
    target = base.joinpath(*parts.parts)
    if not target.resolve().is_relative_to(base.resolve()):
        raise ValueError("source path escapes its cache")
    return target


def publish(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        if read_bounded(path, len(data)) != data:
            raise ValueError(f"existing source differs; retained for review: {path}")
        return
    temporary = path.with_name(path.name + ".pending-" + uuid.uuid4().hex)
    try:
        with temporary.open("xb") as output:
            output.write(data)
        # A single collector owns this generated source cache. Never replace an
        # existing different source on a later collection or repair attempt.
        try:
            os.link(temporary, path)
        except FileExistsError:
            if read_bounded(path, len(data)) != data:
                raise ValueError("source appeared with different contents")
    finally:
        if temporary.exists():
            temporary.unlink()


def fetch(url, limit=LIMIT):
    if OFFLINE:
        raise RuntimeError("source is unavailable in the offline cache")
    if not url.startswith("https://"):
        raise ValueError("source downloads require HTTPS")
    request = urllib.request.Request(url, headers={"User-Agent": "OrbWin-source-inventory/1"})
    with urllib.request.urlopen(request, timeout=25) as response:
        if not response.geturl().startswith("https://"):
            raise ValueError("source redirect downgraded HTTPS")
        data = response.read(limit + 1)
    if len(data) > limit:
        raise ValueError("source exceeds its byte budget")
    return data


def recipe_inputs(text):
    """Read the checksum list and literal URL substitutions, never shell code."""
    variables = dict(re.findall(r"^([A-Za-z_][A-Za-z0-9_]*)=([A-Za-z0-9_.+-]+)$", text, re.M))
    sums = re.search(r'^sha512sums="(.*?)"', text, re.M | re.S)
    inputs = {}
    if sums:
        for line in sums.group(1).strip().splitlines():
            match = re.fullmatch(r"([a-f0-9]{128})\s+(\S+)", line.strip())
            if not match:
                raise ValueError("unsupported or unverified APKBUILD checksum")
            checksum, name = match.groups()
            local_path(pathlib.Path("cache"), name)
            if name in inputs:
                raise ValueError("duplicate source checksum")
            inputs[name] = {"sha512": checksum}
    source = re.search(r'^source="(.*?)"', text, re.M | re.S)
    if source:
        for token in source.group(1).split():
            if "://" not in token:
                continue
            if "$(" in token or "`" in token:
                raise ValueError("shell execution cannot select source URLs")
            def expand(match):
                key = match.group(1) or match.group(2)
                if key not in variables:
                    raise ValueError(f"nonliteral source URL variable: {key}")
                return variables[key]
            token = re.sub(r"\$\{([A-Za-z_]\w*)\}|\$([A-Za-z_]\w*)", expand, token)
            if "$" in token:
                raise ValueError("unsupported source URL expression")
            if "::" in token:
                name, url = token.split("::", 1)
            else:
                url = token
                name = pathlib.PurePosixPath(urllib.parse.urlsplit(url).path).name
            if not url.startswith("https://") or name not in inputs:
                raise ValueError("remote source lacks a pinned HTTPS checksum")
            if inputs[name].get("url") not in (None, url):
                raise ValueError("ambiguous source locations for one recipe input")
            inputs[name]["url"] = url
    if len(inputs) > 256:
        raise ValueError("too many recipe inputs")
    return variables, inputs


def image_packages():
    pin = json.loads((ROOT / "maintenance-image.lock.json").read_text())
    data = read_bounded(ROOT / "artifacts/backend/orbwin-maintenance.tar", 8 << 20)
    if len(data) != pin["bytes"] or digest(data) != pin["sha256"]:
        raise ValueError("maintenance image differs from its reviewed pin")
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        manifest = json.load(archive.extractfile("manifest.json"))
        if len(manifest) != 1 or len(manifest[0]["Layers"]) != 1:
            raise ValueError("review image package extraction after a layer change")
        layer = archive.extractfile(manifest[0]["Layers"][0]).read()
    with tarfile.open(fileobj=io.BytesIO(layer), mode="r:*") as archive:
        info = archive.getmember("lib/apk/db/installed")
        if not info.isfile() or info.size > 1 << 20:
            raise ValueError("invalid package database")
        text = archive.extractfile(info).read().decode()
    packages = []
    for block in text.strip().split("\n\n"):
        fields = {line[:1]: line[2:] for line in block.splitlines() if len(line) > 2 and line[1] == ":"}
        package = {key: fields.get(code) for key, code in [("name", "P"), ("version", "V"), ("license", "L"), ("origin", "o"), ("commit", "c")]}
        if not re.fullmatch(r"[a-z0-9+-]+", package["origin"] or "") or not re.fullmatch(r"[a-f0-9]{40}", package["commit"] or ""):
            raise ValueError("package has no pinned origin/revision")
        packages.append(package)
    if len(packages) > 256:
        raise ValueError("too many image packages")
    return pin, packages


def collect_origin(cache, origin, commit, package_versions):
    base = cache / "aports" / origin
    records = []
    def directory(relative, depth=0):
        if depth > 4:
            raise ValueError("source directory depth exceeds budget")
        url = f"https://api.github.com/repos/alpinelinux/aports/contents/main/{origin}{relative}?ref={commit}"
        metadata = cache / "metadata" / (origin + "-" + commit + relative.replace("/", "_") + ".json")
        listing = json.loads(read_bounded(metadata, 2 << 20) if metadata.exists() else fetch(url, 2 << 20))
        if not isinstance(listing, list) or len(listing) > 256:
            raise ValueError("unexpected aports directory response")
        publish(metadata, json.dumps(listing, sort_keys=True).encode())
        for entry in listing:
            if not entry["path"].startswith("main/" + origin + "/"):
                raise ValueError("aports entry belongs to another origin")
            name = entry["path"].removeprefix("main/" + origin + "/")
            destination = local_path(base, name)
            if entry["type"] == "dir":
                directory("/" + name, depth + 1)
                continue
            if entry["type"] != "file" or not isinstance(entry.get("size"), int) or entry["size"] > 2 << 20:
                raise ValueError("unsupported aports source entry")
            download = f"https://raw.githubusercontent.com/alpinelinux/aports/{commit}/main/{origin}/{urllib.parse.quote(name, safe='/@+.-_')}"
            data = read_bounded(destination, 2 << 20) if destination.exists() else fetch(download, 2 << 20)
            blob = hashlib.sha1(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()
            if blob != entry["sha"]:
                raise ValueError("aports blob differs from its pinned directory")
            publish(destination, data)
            records.append({"file": str(destination.relative_to(cache)).replace("\\", "/"), "url": download, "gitBlob": blob, "sha256": digest(data), "bytes": len(data)})
            if len(records) > 512:
                raise ValueError("too many aports source files")
    directory("")
    variables, inputs = recipe_inputs((base / "APKBUILD").read_text())
    if package_versions != {variables["pkgver"] + "-r" + variables["pkgrel"]}:
        raise ValueError("recipe version differs from the installed packages")
    source_records = []
    for name, item in inputs.items():
        local = local_path(base, name)
        destination = local if "url" not in item else local_path(cache / "distfiles" / origin, name)
        attempts = []
        if destination.exists():
            data = read_bounded(destination)
            if digest(data, "sha512") != item["sha512"]:
                raise ValueError(f"cached recipe input differs: {name}")
            selected = item.get("url", "aports:" + commit)
        else:
            if "url" not in item:
                raise ValueError(f"local recipe input missing from snapshot: {name}")
            for url in [item["url"], "https://distfiles.alpinelinux.org/distfiles/v3.21/" + urllib.parse.quote(name)]:
                try:
                    data = fetch(url)
                    if digest(data, "sha512") != item["sha512"]:
                        raise ValueError("source checksum differs")
                    selected = url
                    publish(destination, data)
                    break
                except Exception as error:
                    attempts.append({"url": url, "error": str(error)})
            else:
                raise ValueError(f"source unavailable: {name}: {attempts}")
        source_records.append({"file": str(destination.relative_to(cache)).replace("\\", "/"), "source": selected, "recipeSource": item.get("url"), "sha512": item["sha512"], "sha256": digest(data), "bytes": len(data), "attempts": attempts})
    return {"origin": origin, "commit": commit, "version": next(iter(package_versions)), "recipeFiles": records, "inputs": source_records, "success": True}


def main():
    global OFFLINE
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cache", type=pathlib.Path, default=ROOT / "artifacts/vendor/maintenance-image")
    parser.add_argument("--offline", action="store_true", help="verify/use cached inputs without network access")
    args = parser.parse_args()
    OFFLINE = args.offline
    cache = args.cache.resolve()
    if not cache.is_relative_to((ROOT / "artifacts").resolve()):
        raise ValueError("source cache must stay in workspace artifacts")
    pin, packages = image_packages()
    origins = {}
    for package in packages:
        value = origins.setdefault(package["origin"], {"commit": package["commit"], "versions": set()})
        if value["commit"] != package["commit"]:
            raise ValueError("multiple revisions for one package origin")
        value["versions"].add(package["version"])
    # The BusyBox recipe explicitly links utmps' static pkg-config libraries.
    # Retain those two source/build contexts at that same aports revision too.
    # This records context, not a claim that binary build logs reveal every
    # historical build-tool version or that the image has been reproduced.
    busybox_commit = origins["busybox"]["commit"]
    for origin in ("utmps", "skalibs"):
        url = f"https://raw.githubusercontent.com/alpinelinux/aports/{busybox_commit}/main/{origin}/APKBUILD"
        path = cache / "aports" / origin / "APKBUILD"
        data = read_bounded(path, 2 << 20) if path.exists() else fetch(url, 2 << 20)
        publish(cache / "aports" / origin / "APKBUILD", data)
        variables, _ = recipe_inputs(data.decode())
        origins[origin] = {"commit": busybox_commit, "versions": {variables["pkgver"] + "-r" + variables["pkgrel"]}, "role": "static-build-context"}
    def run(item):
        origin, value = item
        try:
            result = collect_origin(cache, origin, value["commit"], value["versions"])
            result["role"] = value.get("role", "installed-package-origin")
        except Exception as error:
            result = {"origin": origin, "commit": value["commit"], "success": False, "error": str(error)}
        print(json.dumps({key: result.get(key) for key in ("origin", "success", "error")}), flush=True)
        return result
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        results = list(pool.map(run, sorted(origins.items())))
    report = {"schema": 1, "imageRoot": pin["root"], "imageSha256": pin["sha256"], "packages": packages, "origins": results, "success": all(item["success"] for item in results)}
    (cache / "collection.json").write_text(json.dumps(report, indent=2))
    if not report["success"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
