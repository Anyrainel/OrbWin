import test from "node:test";
import assert from "node:assert/strict";
import {
  sourceProfile,
  linuxDrivePath,
  dockerCommand,
  machineCommand,
  guardMachine,
  guardDaemon,
  guardReview,
  guardBase,
  sourceCopyPath,
  sdkSourceArgs,
  preservationSnapshot,
  projectConfigPaths,
  rancherSelection,
  nativeApplicationEnvelope,
  guardNativeApplicationResponse,
  guardFixtureNamespace,
} from "./migration-project-source.mjs";
import { expectedIDs } from "./qualify-migration-draft.mjs";
const desktop = sourceProfile("docker-desktop"),
  rancher = sourceProfile("rancher-desktop");
const assets = {
  docker: String.raw`F:\runtime\bin\docker.exe`,
  linuxDocker: String.raw`F:\runtime\linux\docker`,
  config: String.raw`F:\fixture folder\anonymous`,
  wsl: String.raw`C:\Windows\System32\wsl.exe`,
};
test("Rancher transport has fixed registration/root/env/socket and Windows cp becomes /mnt/f literal", () => {
  const args = ["cp", "/mnt/f/fixture folder/server", "abc:/orbwin-server"];
  const command = dockerCommand("rancher", assets, args);
  assert.equal(command.binary, assets.wsl);
  assert.deepEqual(command.args, [
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
    "/mnt/f/runtime/linux/docker",
    "--config",
    "/mnt/f/fixture folder/anonymous",
    "--host",
    "unix:///mnt/wsl/rancher-desktop/run/docker.sock",
    ...args,
  ]);
  assert.equal(
    sourceCopyPath(rancher, String.raw`F:\fixture folder\server`),
    "/mnt/f/fixture folder/server",
  );
  assert.equal(
    sourceCopyPath(desktop, String.raw`F:\fixture folder\server`),
    String.raw`F:\fixture folder\server`,
  );
  assert.equal(
    linuxDrivePath(String.raw`F:/unicode 例/file`),
    "/mnt/f/unicode 例/file",
  );
  assert.deepEqual(machineCommand(assets).args.slice(-2), [
    "/bin/cat",
    "/etc/machine-id",
  ]);
  assert.equal(
    dockerCommand("desktop", assets, ["info"]).args[3],
    "npipe:////./pipe/dockerDesktopLinuxEngine",
  );
  assert.equal(
    dockerCommand("owned", assets, ["info"]).args[3],
    "npipe:////./pipe/docker-rancher-desktop-orbwin",
  );
});
test("unsupported sources, foreign machine/daemon/tuple and unsafe host paths refuse", () => {
  for (const name of [undefined, "default", "rancher", "owned"])
    assert.throws(() => sourceProfile(name));
  for (const path of [
    "relative",
    String.raw`F:relative`,
    String.raw`\\server\share\file`,
    String.raw`F:\x\..\file`,
    String.raw`F:\x\file:stream`,
    String.raw`F:\x\evil\nfile`.replace("\\n", "\n"),
    String.raw`F:\x\0`.replace("\\0", "\0"),
  ])
    assert.throws(() => linuxDrivePath(path), path);
  assert.throws(() => dockerCommand("foreign", assets, []));
  for (const args of [
    ["--host", "foreign", "info"],
    ["context", "use", "foreign"],
    ["system", "prune"],
  ])
    assert.throws(() => dockerCommand("rancher", assets, args));
  assert.throws(() => guardMachine("foreign"));
  guardMachine(rancherSelection.machine + "\n");
  assert.throws(() =>
    guardDaemon("rancher", { ID: expectedIDs.desktop, OSType: "linux" }),
  );
  assert.throws(() =>
    guardDaemon("owned", { ID: expectedIDs.owned, OSType: "windows" }),
  );
  const review = {
    target: expectedIDs.owned,
    source: {
      kind: "rancher-desktop",
      daemon: expectedIDs.rancher,
      wsl: { ...rancherSelection },
    },
  };
  guardReview(rancher, review, expectedIDs.owned);
  for (const change of [
    (v) => (v.source.wsl.machine = "foreign"),
    (v) => (v.source.wsl.registration = "foreign"),
    (v) => (v.source.kind = "docker-desktop"),
    (v) => (v.target = expectedIDs.desktop),
  ]) {
    const v = structuredClone(review);
    change(v);
    assert.throws(() => guardReview(rancher, v, expectedIDs.owned));
  }
  assert.throws(() =>
    sourceCopyPath(
      { ...rancher, registration: "foreign" },
      String.raw`F:\file`,
    ),
  );
});
test("SDK commands preserve Desktop ordering and explicitly carry Rancher tuple for all source operations", () => {
  const fields = {
    target: expectedIDs.owned,
    source: expectedIDs.rancher,
    image: "sha256:" + "a".repeat(64),
    reference: "fixture:app",
    destination: String.raw`F:\archive`,
    volume: "fixture-volume",
    fingerprint: "fp",
  };
  assert.deepEqual(sdkSourceArgs(rancher, "review", fields), [
    "migration-rancher-review",
    expectedIDs.owned,
    rancherSelection.registration,
    rancherSelection.machine,
  ]);
  const tuple = [
    expectedIDs.owned,
    expectedIDs.rancher,
    rancherSelection.registration,
    rancherSelection.machine,
  ];
  assert.deepEqual(sdkSourceArgs(rancher, "image-export", fields), [
    "migration-rancher-image-export",
    ...tuple,
    fields.image,
    fields.reference,
    fields.destination,
  ]);
  assert.deepEqual(sdkSourceArgs(rancher, "volume-review", fields), [
    "migration-rancher-volume-review",
    ...tuple,
    fields.volume,
  ]);
  assert.deepEqual(sdkSourceArgs(rancher, "volume-export", fields), [
    "migration-rancher-volume-export",
    ...tuple,
    fields.volume,
    fields.fingerprint,
    fields.destination,
  ]);
  assert.deepEqual(
    sdkSourceArgs(desktop, "volume-export", {
      ...fields,
      source: expectedIDs.desktop,
    }),
    [
      "migration-volume-export",
      "docker-desktop",
      expectedIDs.owned,
      expectedIDs.desktop,
      fields.volume,
      fields.fingerprint,
      fields.destination,
    ],
  );
  assert.throws(() => sdkSourceArgs(rancher, "export-all", fields));
  assert.throws(() =>
    sdkSourceArgs(rancher, "volume-export", {
      ...fields,
      target: expectedIDs.desktop,
    }),
  );
  assert.throws(() =>
    sdkSourceArgs(rancher, "image-export", {
      ...fields,
      source: expectedIDs.desktop,
    }),
  );
});
test("cached source requires exact image/linux/amd64/no Volumes or entrypoint", () => {
  const base = {
    Id: rancher.baseImage,
    Os: "linux",
    Architecture: "amd64",
    Config: { Volumes: null, Entrypoint: null },
  };
  guardBase(rancher, base);
  for (const change of [
    (v) => (v.Id = desktop.baseImage),
    (v) => (v.Os = "windows"),
    (v) => (v.Architecture = "arm64"),
    (v) => (v.Config.Volumes = { "/data": {} }),
    (v) => (v.Config.Entrypoint = ["unexpected"]),
  ]) {
    const v = structuredClone(base);
    change(v);
    assert.throws(() => guardBase(rancher, v));
  }
});
test("all-three canonical snapshots preserve complete network metadata and detect meaningful change", () => {
  let internal = false;
  const raw = (engine, args) => {
    if (args[0] === "info")
      return JSON.stringify({ ID: expectedIDs[engine], OSType: "linux" });
    if (args[0] === "network" && args[1] === "ls") return "net";
    if (args[0] === "network" && args[1] === "inspect")
      return JSON.stringify([
        {
          Id: "net",
          Name: "existing",
          Internal: internal,
          Attachable: true,
          EnableIPv6: false,
          IPAM: { Driver: "default", Config: [{ Subnet: "172.18.0.0/16" }] },
          Options: { custom: "keep" },
          Labels: { owner: "retain" },
          Containers: {},
          Peers: [{ Name: "peer", IP: "1.2.3.4" }],
          FutureField: { nested: "retain" },
        },
      ]);
    if (["ps", "volume", "image"].includes(args[0])) return "";
    throw Error("Unexpected snapshot query " + args);
  };
  const before = preservationSnapshot(raw, []);
  assert.deepEqual(Object.keys(before.engines), [
    "owned",
    "desktop",
    "rancher",
  ]);
  assert.deepEqual(JSON.parse(JSON.stringify(before)), before);
  assert.equal(
    before.engines.rancher.networks[0].inspect.FutureField.nested,
    "retain",
  );
  internal = true;
  const after = preservationSnapshot(raw, []);
  assert.notDeepEqual(after, before);
  assert.equal(after.engines.rancher.networks[0].inspect.Internal, true);
  const configs = projectConfigPaths(
    String.raw`C:\Users\Fixture`,
    String.raw`C:\Users\Fixture\AppData\Local`,
  );
  assert.equal(configs.length, 6);
  assert.ok(configs.some((p) => p.includes("wsl-projects")));
  assert.ok(configs.some((p) => p.includes("wsl-docker")));
});
test("optional native bridge permits only hash-bound Validate/Up/Stop and rejects unknown outcome", () => {
  const nonce = "a".repeat(32),
    hash = "b".repeat(64);
  assert.deepEqual(nativeApplicationEnvelope(nonce, "up", hash, hash), {
    schema: 1,
    nonce,
    expectedTarget: expectedIDs.owned,
    baseSha256: hash,
    mappingSha256: hash,
    action: "up",
  });
  for (const action of ["down", "pull", "build", "restart"])
    assert.throws(() => nativeApplicationEnvelope(nonce, action, hash, hash));
  assert.throws(() => nativeApplicationEnvelope("foreign", "up", hash, hash));
  assert.throws(() => nativeApplicationEnvelope(nonce, "up", "bad", hash));
  const response = {
    schema: 1,
    nonce,
    success: true,
    filesUnchanged: true,
    targetAfter: { Ok: { ID: expectedIDs.owned } },
    result: { Ok: { outcome: { success: true } } },
  };
  guardNativeApplicationResponse(response, nonce);
  for (const change of [
    (v) => (v.success = false),
    (v) => (v.filesUnchanged = false),
    (v) => (v.targetAfter.Ok.ID = expectedIDs.desktop),
    (v) => delete v.result,
  ]) {
    const v = structuredClone(response);
    change(v);
    assert.throws(() => guardNativeApplicationResponse(v, nonce));
  }
});

test("namespace guard refuses preexisting images/volumes/containers/networks before any fixture adoption", () => {
  const project = "orbwin-project-" + "a".repeat(32),
    alias = project + ":app";
  const empty = () => ({
    engines: {
      owned: { containers: [], volumes: [], images: [], networks: [] },
      desktop: { containers: [], volumes: [], images: [], networks: [] },
      rancher: { containers: [], volumes: [], images: [], networks: [] },
    },
  });
  guardFixtureNamespace(empty(), project, alias);
  for (const [kind, value] of [
    ["containers", { name: "/" + project + "-source" }],
    ["volumes", { name: project + "-source" }],
    ["images", { repoTags: [alias] }],
    ["networks", { name: project + "_default" }],
  ]) {
    const snapshot = empty();
    snapshot.engines.rancher[kind].push(value);
    assert.throws(() => guardFixtureNamespace(snapshot, project, alias));
  }
});
