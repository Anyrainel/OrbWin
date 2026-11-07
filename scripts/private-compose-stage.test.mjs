import test from "node:test";
import assert from "node:assert/strict";
import {
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, resolve } from "node:path";
import {
  cacheEvidence,
  manifestRequests,
  registryRequestObservations,
  pollManifestEvidence,
  composePlan,
  ownedBuiltImage,
  cleanupPrivateComposeStage,
  builderMetadataAt,
  selectedPluginEvidence,
  metadataDirectoryChain,
  safeStageFailure,
} from "./private-compose-stage.mjs";

test("extended local-drive plugin paths stop at the drive root and failures omit payloads", () => {
  const normal = resolve("artifacts/runtime-tools/docker-cli-plugins");
  const chain = metadataDirectoryChain(`\\\\?\\${normal}`);
  assert.deepEqual(chain, metadataDirectoryChain(normal));
  assert.ok(chain.every((path) => !/^[A-Za-z]:$/.test(path)));
  const error = Object.assign(Error("secret path and config payload"), {
    code: "EISDIR",
  });
  const result = safeStageFailure(error, {
    mode: "authenticated",
    stage: "verify-tools",
  });
  assert.equal(result.code, "EISDIR");
  assert.equal(result.stage, "verify-tools");
  assert.ok(!JSON.stringify(result).includes("secret"));
});

test("metadata observation refuses root/traversal junctions and oversized selection before reading", () => {
  const temp = mkdtempSync(resolve(tmpdir(), "orbwin-compose-metadata-test-"));
  try {
    const root = resolve(temp, "buildx"),
      target = resolve(temp, "target");
    mkdirSync(root);
    mkdirSync(target);
    writeFileSync(resolve(root, "current"), "metadata");
    assert.equal(typeof builderMetadataAt(root).selection.current, "string");
    writeFileSync(resolve(root, "current"), Buffer.alloc(65537));
    assert.throws(() => builderMetadataAt(root), /64KiB/);
    writeFileSync(resolve(root, "current"), "metadata");
    symlinkSync(target, resolve(root, "refs"), "junction");
    assert.throws(() => builderMetadataAt(root), /link/);
    symlinkSync(root, resolve(temp, "linked-root"), "junction");
    assert.throws(
      () => builderMetadataAt(resolve(temp, "linked-root")),
      /link/,
    );
    rmSync(resolve(root, "refs"));
    symlinkSync(resolve(temp, "missing"), resolve(root, "refs"), "junction");
    assert.throws(() => builderMetadataAt(root), /link/);
  } finally {
    assert.equal(dirname(temp), resolve(tmpdir()));
    assert.ok(basename(temp).startsWith("orbwin-compose-metadata-test-"));
    rmSync(temp, { recursive: true });
  }
});
test("plugin proof hashes discovered path rather than assuming adjacent staged plugin selection", () => {
  const paths = [
    resolve("discovered/docker-compose.exe"),
    resolve("discovered/docker-buildx.exe"),
  ];
  const plugins = ["compose", "buildx"].map((Name, index) => ({
    Name,
    Version: "v1.0",
    Path: paths[index],
  }));
  const tools = ["compose", "buildx"].map((name, index) => ({
    path: `docker-cli-plugins/docker-${name}.exe`,
    version: index === 1 ? "v1.0" : "1.0",
    sha256: String(index).repeat(64),
  }));
  const hashed = [];
  const result = selectedPluginEvidence(plugins, tools, (path) => {
    hashed.push(path);
    return tools[paths.indexOf(path)].sha256;
  });
  assert.deepEqual(hashed, paths);
  assert.ok(!JSON.stringify(result).includes("discovered"));
  const wrongVersion = structuredClone(plugins);
  wrongVersion[1].Version = "v1.1";
  assert.throws(() => selectedPluginEvidence(wrongVersion, tools,
    path => tools[paths.indexOf(path)].sha256));
  assert.throws(() =>
    selectedPluginEvidence(plugins, tools, () => "unexpected"),
  );
  assert.throws(() =>
    selectedPluginEvidence([...plugins, plugins[0]], tools, () => "unexpected"),
  );
});
const nonce = "a".repeat(32),
  fullId = `sha256:${"b".repeat(64)}`;
function fixture() {
  const state = {
    nonce,
    name: `orbwin-private-client-${nonce}`,
    server: "127.0.0.1:52345",
    image: `127.0.0.1:52345/fixture/image:${nonce}`,
    before: { inventories: { owned: { images: [], containers: [] } } },
    committedRootFS: { Layers: [`sha256:${"c".repeat(64)}`] },
    intents: [],
    wrapper: "proof-only",
  };
  const plan = composePlan(state);
  const image = {
    Id: fullId,
    RepoTags: [plan.tag],
    Config: {
      Labels: {
        "dev.orbwin.qualification": nonce,
        "dev.orbwin.private-compose": plan.marker,
      },
      Cmd: [
        "/bin/busybox",
        "timeout",
        "-s",
        "KILL",
        "120",
        "/bin/busybox",
        "sleep",
        "120",
      ],
    },
    RootFS: {
      Layers: [...state.committedRootFS.Layers, `sha256:${"d".repeat(64)}`],
    },
    Os: "linux",
    Architecture: "amd64",
  };
  return { state, plan, image };
}
test("private Compose ownership refuses pre-existing, retagged, replaced and unbounded images", () => {
  const original = fixture();
  assert.equal(
    ownedBuiltImage(original.image, original.state, original.plan),
    fullId,
  );
  for (const change of [
    ({ state }) => state.before.inventories.owned.images.push(fullId),
    ({ image }) => image.RepoTags.push("user-owned:important"),
    ({ image }) =>
      (image.Config.Labels["dev.orbwin.qualification"] = "another-owner"),
    ({ image }) =>
      (image.Config.Labels["dev.orbwin.private-compose"] = "another-build"),
    ({ image }) => (image.RootFS.Layers[0] = `sha256:${"e".repeat(64)}`),
    ({ image }) => (image.Config.Cmd = ["/bin/sh"]),
    ({ image }) => (image.Config.Volumes = { "/user-data": {} }),
  ]) {
    const current = fixture();
    change(current);
    assert.throws(() =>
      ownedBuiltImage(current.image, current.state, current.plan),
    );
  }
});
test("uncertain cleanup reads ownership before emitting a destructive command", () => {
  const { state, plan, image } = fixture();
  image.RepoTags.push("keep-this:tag");
  state.privateCompose = { outputTags: [plan.tag] };
  const calls = [];
  const ctx = {
    state,
    save() {},
    execute(_binary, args) {
      calls.push(args);
      if (args[0] === "ps") return { stdout: "", status: 0 };
      if (args[0] === "image" && args[1] === "ls")
        return { stdout: fullId, status: 0 };
      if (args[0] === "image" && args[1] === "inspect")
        return { stdout: JSON.stringify([image]), status: 0 };
      throw Error("Unexpected command");
    },
  };
  assert.throws(() => cleanupPrivateComposeStage(ctx));
  assert.ok(calls.length > 0);
  assert.ok(
    calls.every(
      (args) =>
        !args.includes("rm") &&
        !args.includes("stop") &&
        !args.includes("prune"),
    ),
  );
});
test("cache accounting preserves formatted sizes and refuses unknown counter schemas", () => {
  const du = {
    ID: "record-1",
    Size: "4.096kB",
    UsageCount: 2,
    Mutable: false,
    Reclaimable: true,
    Shared: false,
    Description: "unrelated sensitive project path",
  };
  const history = {
    ref: "default/default/reference",
    status: "Completed",
    cached_steps: 1,
    completed_steps: 2,
    total_steps: 2,
    name: "unrelated project",
  };
  const result = cacheEvidence(JSON.stringify(du), JSON.stringify(history));
  assert.equal(result.du[0].reportedSize, "4.096kB");
  assert.equal(result.history[0].status, "Completed");
  assert.ok(!JSON.stringify(result).includes("unrelated"));
  assert.ok(!JSON.stringify(result).includes(history.ref));
  for (const update of [
    { Size: 4096 },
    { UsageCount: -1 },
    { Shared: "false" },
    { ID: undefined },
  ])
    assert.throws(() =>
      cacheEvidence(
        JSON.stringify({ ...du, ...update }),
        JSON.stringify(history),
      ),
    );
  assert.throws(() =>
    cacheEvidence(JSON.stringify([du, du]), JSON.stringify(history)),
  );
  assert.throws(() =>
    cacheEvidence(
      JSON.stringify(du),
      JSON.stringify({ ...history, status: "new-unknown-state" }),
    ),
  );
});
test("manifest observations require exact nonce path, response status and HTTP method", () => {
  const path = `/v2/fixture/image/manifests/${nonce}`;
  const json = JSON.stringify({
    "http.request.uri": path,
    "http.request.method": "HEAD",
    "http.response.status": 200,
  });
  const text = `time="2026-10-25T22:00:00Z" http.request.uri="${path}" http.request.method=GET http.response.status=401`;
  const distractors = [
    json.replace(nonce, "e".repeat(32)),
    json.replace("HEAD", "POST"),
    json.replace(`\"${path}\"`, `\"${path}/extra\"`),
    JSON.stringify({ "http.request.uri": path, "http.request.method": "GET" }),
  ];
  assert.deepEqual(
    manifestRequests(
      [`2026-10-25T22:00:00.000Z ${json}`, text, ...distractors].join("\n"),
      nonce,
    ),
    { success: 1, refused: 1 },
  );
});
test("registry access evidence handles CLF, digest references and queries without retaining headers", () => {
  const manifest = `sha256:${"e".repeat(64)}`;
  const tag = `/v2/fixture/image/manifests/${nonce}`;
  const known = `/v2/fixture/image/manifests/${manifest}`;
  const clf = (path, status, method = "HEAD") =>
    `2026-10-25T22:00:00.000Z 127.0.0.1 - synthetic-user [25/Oct/2026:22:00:00 +0000] "${method} ${path} HTTP/1.1" ${status} 0 "sensitive-referrer" "sensitive-user-agent"`;
  const text = [
    clf(`${tag}?ns=127.0.0.1%3A5000&secret=should-never-persist`, 200),
    clf(known.replace("sha256:", "sha256%3A"), 401),
    clf(`/v2/fixture/image/manifests/sha256:${"f".repeat(64)}`, 200),
    clf(`/v2/unrelated/manifests/${nonce}`, 200),
    clf("/v2/", 401),
    clf(`${tag}/extra`, 200),
  ].join("\n");
  assert.deepEqual(manifestRequests(text, nonce, manifest), {
    success: 1,
    refused: 1,
  });
  const records = registryRequestObservations(text, nonce, manifest);
  assert.equal(records.length, 4);
  assert.equal(records[0].format, "combined-access");
  assert.equal(records[0].queryPresent, true);
  assert.equal(records[1].pathClass, "known-manifest-digest");
  assert.equal(records[2].pathClass, "other-manifest-digest");
  const receipt = JSON.stringify(records);
  for (const payload of [
    "secret",
    "should-never",
    "sensitive",
    "synthetic-user",
    "unrelated",
    manifest,
  ])
    assert.ok(!receipt.includes(payload));
});
test("bounded log-flush polling retains sanitized observations and never injects registry requests", async () => {
  const calls = [];
  const state = {
    nonce,
    registryContainer: "a".repeat(64),
    wrapper: "proof",
    privateCompose: {},
  };
  const ctx = {
    state,
    save() {},
    execute(_binary, args, options) {
      calls.push(args);
      assert.equal(args[0], "logs");
      assert.equal(options.timeout, 2000);
      return {
        stdout:
          calls.length === 1
            ? "unrelated logger diagnostic"
            : `127.0.0.1 - - [25/Oct/2026:22:00:00 +0000] "HEAD /v2/fixture/image/manifests/${nonce}?ns=loopback HTTP/1.1" 200 0 "-" "-"`,
        stderr: "",
        status: 0,
      };
    },
  };
  assert.deepEqual(
    await pollManifestEvidence(ctx, { success: 0, refused: 0 }, false),
    { success: 1, refused: 0 },
  );
  assert.equal(calls.length, 2);
  assert.equal(state.privateCompose.observationSamples.length, 2);
  assert.equal(
    state.privateCompose.observationSamples[1].sanitizedRequests[0].pathClass,
    "nonce-manifest-tag",
  );
});
test("private Compose plan keeps a forced base lookup and finite isolated workload", () => {
  const { state, plan } = fixture();
  const app = plan.compose.services.app;
  assert.equal(app.build.pull, true);
  assert.equal(app.build.no_cache, true);
  assert.equal(app.build.network, "none");
  assert.equal(app.network_mode, "none");
  assert.equal(app.read_only, true);
  assert.equal(app.volumes, undefined);
  assert.equal(app.ports, undefined);
  assert.ok(plan.dockerfile.startsWith(`FROM ${state.image}\n`));
  assert.ok(!plan.dockerfile.includes("# syntax="));
  assert.throws(() => composePlan({ ...state, image: "public:latest" }));
  assert.throws(() =>
    composePlan({ ...state, server: "untrusted.example:80" }),
  );
});
