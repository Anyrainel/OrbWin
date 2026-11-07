// Pure transport/identity/snapshot contracts for the opt-in qualification harness.
import assert from "node:assert/strict";
import { win32 as windows } from "node:path";
import { createHash } from "node:crypto";
import {
  canonicalEngineSnapshot,
  configFingerprint,
  expectedIDs,
} from "./qualify-migration-draft.mjs";
export const rancherSelection = Object.freeze({
  registration: "231810b0-fa21-48a8-b0a7-86f9a1360029",
  machine: "5563914b91c14609814e3657a227cff1",
});
const endpoints = {
  owned: "npipe:////./pipe/docker-rancher-desktop-orbwin",
  desktop: "npipe:////./pipe/dockerDesktopLinuxEngine",
  rancher: "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
};
export function sourceProfile(kind) {
  assert.ok(
    ["docker-desktop", "rancher-desktop"].includes(kind),
    "Explicit supported source selection required",
  );
  const engine = kind === "docker-desktop" ? "desktop" : "rancher";
  return Object.freeze({
    kind,
    engine,
    daemon: expectedIDs[engine],
    endpoint: endpoints[engine],
    ...(engine === "rancher" ? rancherSelection : {}),
    baseImage:
      engine === "rancher"
        ? "sha256:101b4afd76732482eff9b95cae5f94bcf295e521fbec4e01b69c5421f3f3f3e5"
        : "sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6",
  });
}
function checkedProfile(profile) {
  assert.deepEqual(
    profile,
    sourceProfile(profile.kind),
    "Source profile/WSL tuple override refused",
  );
}
export function linuxDrivePath(file) {
  assert.equal(typeof file, "string");
  assert.match(
    file,
    /^[A-Za-z]:[\\/]/,
    "Absolute local Windows drive path required",
  );
  assert.ok(
    !/[\0\r\n]/.test(file) && !file.slice(2).includes(":"),
    "Control/ADS path refused",
  );
  assert.ok(
    !file
      .slice(3)
      .split(/[\\/]/)
      .some((p) => p === "." || p === ".."),
    "Dot segments refused",
  );
  const value = windows.normalize(file);
  return (
    "/mnt/" + value[0].toLowerCase() + value.slice(2).replaceAll("\\", "/")
  );
}
function wslPrefix() {
  return [
    "--distribution-id",
    "{" + rancherSelection.registration + "}",
    "--user",
    "root",
    "--exec",
    "/usr/bin/env",
    "-i",
    "PATH=/usr/sbin:/usr/bin:/sbin:/bin",
    "LANG=C",
    "HOME=/nonexistent",
  ];
}
export function machineCommand(assets) {
  return {
    binary: assets.wsl,
    args: [...wslPrefix(), "/bin/cat", "/etc/machine-id"],
  };
}
export function dockerCommand(engine, assets, args) {
  assert.ok(Object.hasOwn(endpoints, engine), "Unknown engine selection");
  assert.ok(
    Array.isArray(args) &&
      args.every((v) => typeof v === "string" && !v.includes("\0")),
    "Literal Docker argv required",
  );
  assert.ok(
    [
      "info",
      "ps",
      "create",
      "start",
      "stop",
      "rm",
      "commit",
      "cp",
      "volume",
      "image",
      "container",
      "inspect",
      "network",
      "logs",
      "compose",
    ].includes(args[0]),
    "Only fixed fixture Docker command families are admitted",
  );
  if (engine === "rancher")
    return {
      binary: assets.wsl,
      args: [
        ...wslPrefix(),
        linuxDrivePath(assets.linuxDocker),
        "--config",
        linuxDrivePath(assets.config),
        "--host",
        endpoints.rancher,
        ...args,
      ],
    };
  return {
    binary: assets.docker,
    args: ["--config", assets.config, "--host", endpoints[engine], ...args],
  };
}
export function guardMachine(value) {
  assert.equal(
    value.trim(),
    rancherSelection.machine,
    "Selected WSL machine changed",
  );
}
export function guardDaemon(engine, info) {
  assert.ok(Object.hasOwn(expectedIDs, engine), "Unknown daemon selection");
  assert.equal(
    info.ID,
    expectedIDs[engine],
    "Explicit daemon identity changed",
  );
  assert.equal(info.OSType, "linux", "Linux daemon required");
}
export function guardReview(profile, review, target) {
  checkedProfile(profile);
  assert.equal(target, expectedIDs.owned);
  assert.equal(review.target, target);
  assert.equal(review.source.kind, profile.kind);
  assert.equal(review.source.daemon, profile.daemon);
  if (profile.engine === "rancher") {
    assert.equal(
      review.source.wsl?.registration,
      rancherSelection.registration,
    );
    assert.equal(review.source.wsl?.machine, rancherSelection.machine);
  }
}
export function guardBase(profile, image) {
  checkedProfile(profile);
  assert.equal(image.Id, profile.baseImage, "Cached immutable base differs");
  assert.equal(image.Os, "linux");
  assert.equal(image.Architecture, "amd64");
  assert.deepEqual(image.Config.Volumes ?? {}, {});
  assert.equal(
    (image.Config.Entrypoint ?? []).length,
    0,
    "Cached base entrypoint must be empty",
  );
}
export function sourceCopyPath(profile, file) {
  checkedProfile(profile);
  return profile.engine === "rancher" ? linuxDrivePath(file) : file;
}
export function sdkSourceArgs(
  profile,
  operation,
  { target, source, image, reference, destination, volume, fingerprint } = {},
) {
  checkedProfile(profile);
  assert.equal(target, expectedIDs.owned, "Wrong target");
  if (operation !== "review")
    assert.equal(source, profile.daemon, "Wrong source");
  const tuple = [
    target,
    source,
    rancherSelection.registration,
    rancherSelection.machine,
  ];
  if (profile.engine === "rancher") {
    switch (operation) {
      case "review":
        return [
          "migration-rancher-review",
          target,
          rancherSelection.registration,
          rancherSelection.machine,
        ];
      case "image-export":
        return [
          "migration-rancher-image-export",
          ...tuple,
          image,
          reference,
          destination,
        ];
      case "volume-review":
        return ["migration-rancher-volume-review", ...tuple, volume];
      case "volume-export":
        return [
          "migration-rancher-volume-export",
          ...tuple,
          volume,
          fingerprint,
          destination,
        ];
      default:
        throw Error("Unsupported SDK source operation");
    }
  }
  switch (operation) {
    case "review":
      return ["migration-review", "docker-desktop", target];
    case "image-export":
      return [
        "migration-image-export",
        "docker-desktop",
        target,
        source,
        image,
        reference,
        destination,
      ];
    case "volume-review":
      return [
        "migration-volume-review",
        "docker-desktop",
        target,
        source,
        volume,
      ];
    case "volume-export":
      return [
        "migration-volume-export",
        "docker-desktop",
        target,
        source,
        volume,
        fingerprint,
        destination,
      ];
    default:
      throw Error("Unsupported SDK source operation");
  }
}
const lines = (s) => s.trim().split(/\r?\n/).filter(Boolean),
  sha = (v) =>
    JSON.stringify(v) === undefined
      ? undefined
      : createHash("sha256").update(JSON.stringify(v)).digest("hex");
export function preservationSnapshot(read, configPaths) {
  const ids = {},
    engines = {};
  for (const engine of ["owned", "desktop", "rancher"]) {
    const json = (args) => JSON.parse(read(engine, args));
    const info = json(["info", "--format={{json .}}"]);
    guardDaemon(engine, info);
    ids[engine] = info.ID;
    const idsFor = (args, limit) => {
      const out = [...new Set(lines(read(engine, args)))];
      assert.ok(out.length <= limit, "Snapshot resource limit exceeded");
      return out;
    };
    const containers = idsFor(["ps", "-aq", "--no-trunc"], 256),
      volumes = idsFor(["volume", "ls", "-q"], 256),
      images = idsFor(["image", "ls", "-aq", "--no-trunc"], 512),
      networks = idsFor(["network", "ls", "-q", "--no-trunc"], 256);
    engines[engine] = canonicalEngineSnapshot({
      containers: (containers.length
        ? json(["container", "inspect", ...containers])
        : []
      ).map((c) => ({
        id: c.Id,
        name: c.Name,
        image: c.Image,
        created: c.Created,
        running: c.State.Running,
        paused: c.State.Paused,
        restarts: c.RestartCount,
        configSha: sha(c.Config),
        hostConfigSha: sha(c.HostConfig),
        mounts: c.Mounts,
      })),
      volumes: (volumes.length
        ? json(["volume", "inspect", ...volumes])
        : []
      ).map((v) => ({
        name: v.Name,
        created: v.CreatedAt,
        driver: v.Driver,
        scope: v.Scope,
        mountpoint: v.Mountpoint,
        labelsSha: sha(v.Labels),
        optionsSha: sha(v.Options),
      })),
      images: (images.length ? json(["image", "inspect", ...images]) : []).map(
        (v) => ({
          id: v.Id,
          repoTags: [...(v.RepoTags ?? [])].sort(),
          repoDigests: [...(v.RepoDigests ?? [])].sort(),
          created: v.Created,
          rootfsSha: sha(v.RootFS),
          configSha: sha(v.Config),
        }),
      ),
      // Preserve the WHOLE network inspect object, including Internal/Attachable,
      // IPv6/IPAM/options/labels/peers/driver-specific fields and live attachments.
      networks: (networks.length
        ? json(["network", "inspect", ...networks])
        : []
      ).map((n) => ({ id: n.Id, name: n.Name, inspect: n })),
    });
  }
  return {
    ids,
    engines,
    configs: configPaths.map((file) => configFingerprint(file)),
  };
}
export function projectConfigPaths(user, local) {
  const home = windows.join(local, "OrbWinData", "home");
  return [
    windows.join(user, ".docker/config.json"),
    windows.join(user, ".kube/config"),
    windows.join(user, ".wslconfig"),
    windows.join(home, ".rdorbwin/docker/config.json"),
    windows.join(home, ".rdorbwin/wsl-docker/config.json"),
    windows.join(
      home,
      ".rdorbwin/wsl-projects/710eb4f4d7f72b0e329a51d30a83377bc7ef6c0cca1a444bd8d2e70e72326e8f.json",
    ),
  ];
}
export function guardFixtureNamespace(snapshot, project, alias) {
  assert.match(project, /^orbwin-project-[a-f0-9]{32}$/);
  assert.equal(alias, project + ":app");
  for (const engine of Object.values(snapshot.engines)) {
    assert.ok(
      !engine.containers.some((v) => v.name?.startsWith("/" + project)),
      "Fixture container namespace already exists",
    );
    assert.ok(
      !engine.volumes.some((v) => v.name?.startsWith(project)),
      "Fixture volume namespace already exists",
    );
    assert.ok(
      !engine.images.some((v) => v.repoTags?.includes(alias)),
      "Fixture image reference already exists",
    );
    assert.ok(
      !engine.networks.some((v) => v.name?.startsWith(project)),
      "Fixture network namespace already exists",
    );
  }
}

export function nativeApplicationEnvelope(
  nonce,
  action,
  baseSha256,
  mappingSha256,
) {
  assert.match(nonce, /^[a-f0-9]{32}$/);
  assert.ok(["validate", "up", "stop"].includes(action));
  assert.match(baseSha256, /^[a-f0-9]{64}$/);
  assert.match(mappingSha256, /^[a-f0-9]{64}$/);
  return {
    schema: 1,
    nonce,
    expectedTarget: expectedIDs.owned,
    baseSha256,
    mappingSha256,
    action,
  };
}
export function guardNativeApplicationResponse(value, nonce) {
  assert.equal(value.schema, 1);
  assert.equal(value.nonce, nonce);
  assert.equal(value.success, true);
  assert.equal(value.filesUnchanged, true);
  assert.equal(value.targetAfter?.Ok?.ID, expectedIDs.owned);
  assert.equal(value.result?.Ok?.outcome?.success, true);
}
