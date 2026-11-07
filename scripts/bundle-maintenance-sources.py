"""Freeze verified image-package sources/notices into an offline companion archive.

This stages source files and license texts only. It executes no upstream source,
and does not claim bit-for-bit reconstruction of the upstream image.
"""
import gzip
import argparse
import importlib.util
import io
import json
import pathlib
import re
import tarfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location("maintenance_collect", ROOT / "scripts/collect-maintenance-sources.py")
collect = importlib.util.module_from_spec(spec)
spec.loader.exec_module(collect)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cache", type=pathlib.Path, default=ROOT / "artifacts/vendor/maintenance-image")
    cache = parser.parse_args().cache.resolve()
    if not cache.is_relative_to((ROOT / "artifacts").resolve()):
        raise ValueError("source cache must stay in workspace artifacts")
    report = json.loads(collect.read_bounded(cache / "collection.json", 2 << 20))
    image, packages = collect.image_packages()
    if not report["success"] or report["imageSha256"] != image["sha256"] or report["packages"] != packages:
        raise ValueError("source collection does not match the pinned image")
    notices_dir = ROOT / "third-party/maintenance-image"
    sources = {}
    origins = []
    notices = {}

    def add_notice(name, data, provenance):
        collect.local_path(notices_dir, name)
        if len(data) > 1 << 20:
            raise ValueError("notice is oversized")
        if name in notices and notices[name]["sha256"] != collect.digest(data):
            raise ValueError("notice filename collision")
        collect.publish(notices_dir / name, data)
        notices[name] = {"file": name, "sha256": collect.digest(data), "bytes": len(data), "source": provenance}

    for origin in report["origins"]:
        if not origin["success"]:
            raise ValueError("an origin is incomplete")
        normalized = {key: origin[key] for key in ("origin", "commit", "version", "role")}
        normalized["recipeFiles"] = sorted(origin["recipeFiles"], key=lambda row: row["file"])
        normalized["inputs"] = []
        for record in origin["recipeFiles"] + origin["inputs"]:
            name = record["file"]
            path = collect.local_path(cache, name)
            data = collect.read_bounded(path)
            if collect.digest(data) != record["sha256"] or len(data) != record["bytes"]:
                raise ValueError("source changed after collection: " + name)
            if record.get("sha512") and collect.digest(data, "sha512") != record["sha512"]:
                raise ValueError("recipe checksum differs: " + name)
            if name in sources and sources[name] != data:
                raise ValueError("source filename collision")
            sources[name] = data
        for record in origin["inputs"]:
            normalized["inputs"].append({key: record[key] for key in ("file", "recipeSource", "sha512", "sha256", "bytes")})
            if not record["recipeSource"]:
                continue
            path = cache / record["file"]
            try:
                with tarfile.open(fileobj=io.BytesIO(sources[record["file"]]), mode="r:*") as archive:
                    count = 0
                    for member in archive:
                        count += 1
                        if count > 100000:
                            raise ValueError("source archive exceeds member budget")
                        name = pathlib.PurePosixPath(member.name)
                        if member.isfile() and len(name.parts) <= 3 and re.match(r"(?i)^(license|copying|copyright|notice)([._-]|$)", name.name):
                            if member.size > 1 << 20:
                                raise ValueError("source notice exceeds its budget")
                            label = origin["origin"] + "-" + "-".join(name.parts[1:])
                            add_notice("LICENSE/" + label + ".txt", archive.extractfile(member).read(), {"archive": record["file"], "member": member.name, "archiveSha512": record["sha512"]})
            except tarfile.ReadError:
                # protocols/services are checksummed data inputs, not archives.
                pass
        origins.append(normalized)

    for name in ("getconf.c", "getent.c", "iconv.c"):
        data = sources["aports/musl/" + name]
        end = data.find(b"*/")
        if end < 0 or end > 16384:
            raise ValueError("review the musl-utils source copyright header")
        add_notice("LICENSE/musl-utils-" + name + ".txt", data[:end + 2] + b"\n", {"source": "aports/musl/" + name, "sha256": collect.digest(data)})
    ca_source = next(row for row in report["origins"] if row["origin"] == "ca-certificates")["inputs"][0]["file"]
    with tarfile.open(fileobj=io.BytesIO(sources[ca_source]), mode="r:*") as archive:
        prefix = "ca-certificates-20261020/"
        for name, lines in (("certdata.txt", 6), ("c_rehash.c", 10)):
            data = archive.extractfile(prefix + name).read()
            add_notice("LICENSE/ca-certificates-" + name + ".txt", b"\n".join(data.splitlines()[:lines]) + b"\n", {"archive": ca_source, "member": prefix + name})

    # Supplement component notices with unmodified standard texts. The MIT text
    # is a template, not an invented grant/copyright holder for another package.
    for license_id in ("MIT", "MPL-2.0"):
        url = f"https://raw.githubusercontent.com/spdx/license-list-data/v3.26.0/text/{license_id}.txt"
        local = cache / "license-texts" / (license_id + ".txt")
        data = collect.read_bounded(local, 1 << 20) if local.exists() else collect.fetch(url, 1 << 20)
        collect.publish(local, data)
        add_notice("LICENSE/SPDX-" + license_id + ".txt", data, {"url": url, "purpose": "unmodified standard license text; component notices remain authoritative"})

    bindings = {}
    for package in packages:
        origin = package["origin"]
        values = [name for name in notices if name.startswith("LICENSE/" + origin + "-")]
        if origin == "musl":
            values = ["LICENSE/musl-COPYRIGHT.txt"]
            if package["name"] == "musl-utils":
                values += [name for name in notices if name.startswith("LICENSE/musl-utils-")] + ["LICENSE/apk-tools-LICENSE.txt"]
        elif origin in ("alpine-base", "alpine-keys"):
            values = ["LICENSE/SPDX-MIT.txt"]
        elif origin == "alpine-baselayout":
            values = ["LICENSE/apk-tools-LICENSE.txt"]
        elif origin == "ca-certificates":
            values += ["LICENSE/SPDX-MIT.txt", "LICENSE/SPDX-MPL-2.0.txt"]
        elif origin == "busybox":
            values += ["LICENSE/utmps-COPYING.txt", "LICENSE/skalibs-COPYING.txt"]
        if not values or any(name not in notices for name in values):
            raise ValueError("package notice binding is incomplete: " + package["name"])
        bindings[package["name"]] = sorted(set(values))

    lines = ["OrbWin bundled Alpine maintenance image", "", "Image: " + image["source"], "Archive SHA-256: " + image["sha256"], "", "The upstream image is unmodified. OrbWin removes image-name metadata from its", "transport archive and loads it by immutable digest. No upstream program is", "executed while collecting these sources or notices.", "", "Package-specific license/copyright texts are in LICENSE/. SOURCE-INVENTORY.json", "records the image packages, exact aports revisions and checked input hashes.", "SOURCES.tar.gz accompanies this notice and contains upstream source archives,", "aports recipes/patches/configuration and this notice inventory. Its paths and", "outer tar modes/timestamps are normalized; enclosed upstream archives are verbatim.", "Use the recorded Alpine abuild recipes/toolchain for the target linux/amd64.", "An identical binary rebuild or a complete historical build-tool inventory has", "not been demonstrated. utmps/skalibs source contexts at the BusyBox recipe's", "revision are included because that recipe explicitly requests static libraries.", "", "Standard MIT/MPL texts are supplementary. MIT template placeholders have not", "been replaced with invented authors or used to replace original copyright", "notices. Component/source notices and package-declared licenses are preserved.", "No separate copyright notices occur in the included Alpine signing-key data;", "their recipe and declared MIT license are retained verbatim. System compiler", "and ordinary build tools are not bundled here. Individual source files may have", "additional notices; their complete archives and patches remain available below.", "", "Installed image packages (declared licenses):"]
    lines += [f"- {p['name']} {p['version']}: {p['license']} (origin {p['origin']}, {p['commit']})" for p in packages]
    lines += ["", "These third-party terms apply to their respective components. They do not", "replace the licensing of OrbWin or other bundled dependencies.", ""]
    add_notice("NOTICE.txt", "\n".join(lines).encode(), {"kind": "original inventory and source-location guidance"})
    frozen = {"schema": 1, "imageRoot": image["root"], "imageSha256": image["sha256"], "packages": packages, "origins": origins, "noticeBindings": bindings, "notices": sorted(notices.values(), key=lambda item: item["file"]), "sourceFiles": [{"file": name, "sha256": collect.digest(data), "bytes": len(data)} for name, data in sorted(sources.items())]}
    source_index = json.dumps(frozen, indent=2, sort_keys=True).encode() + b"\n"
    bundle_files = {**sources, "SOURCE-INVENTORY.json": source_index, **{"notices/" + name: collect.read_bounded(notices_dir / name, 1 << 20) for name in notices}}
    output = io.BytesIO()
    with gzip.GzipFile(filename="", fileobj=output, mode="wb", mtime=0, compresslevel=9) as compressed:
        with tarfile.open(fileobj=compressed, mode="w", format=tarfile.USTAR_FORMAT) as archive:
            for name, data in sorted(bundle_files.items()):
                collect.local_path(pathlib.Path("bundle"), name)
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.mtime = len(data), 0o644, 0
                archive.addfile(info, io.BytesIO(data))
    payload = output.getvalue()
    if len(payload) > 64 << 20:
        raise ValueError("source bundle exceeds its staging budget")
    frozen["sourceArchive"] = {"file": "maintenance-sources.tar.gz", "sha256": collect.digest(payload), "bytes": len(payload)}
    collect.publish(ROOT / "artifacts/backend/maintenance-sources.tar.gz", payload)
    collect.publish(ROOT / "maintenance-sources.lock.json", json.dumps(frozen, indent=2, sort_keys=True).encode() + b"\n")
    print(json.dumps({"packages": len(packages), "origins": len(origins), "sourceFiles": len(sources), "notices": len(notices), "archive": frozen["sourceArchive"]}, indent=2))


if __name__ == "__main__":
    main()
