import test from "node:test";
import assert from "node:assert/strict";
import { resolve } from "node:path";
import { tmpdir } from "node:os";
import {
  devcontainerComposePlan,
  createCallbackBudget,
  generatedDockerfilePath,
  defaultComposeCommandEvidence,
  negativeDevcontainerCommandEvidence,
  classifyNegativeDevcontainerRefusal,
  verifyGeneratedDockerfile,
  ownedDevcontainerComposeImage,
  ownedDevcontainerComposeBase,
  ownedDevcontainerComposeContainer,
  cleanupPrivateDevcontainerCompose,
} from "./private-devcontainer-compose-stage.mjs";

test("early negative requires an exact selected-alias source failure, not unrelated auth output", () => {
  const { state } = fixture(),
    wrapper = resolve("artifacts/package/orbwin-docker.exe");
  const event = (type, text) => JSON.stringify({ type, text });
  const pull = event("start", `Run: ${wrapper} pull ${state.image}`),
    inspect = event(
      "start",
      `Run: ${wrapper} inspect --type image ${state.image}`,
    ),
    failure = event("text", `Command failed: ${wrapper} pull ${state.image}`),
    auth = event(
      "raw",
      "Error response from daemon: unauthorized: authentication required",
    );
  const parse = (text) =>
    negativeDevcontainerCommandEvidence(text, wrapper, state);
  const events = parse([inspect, pull, failure, auth, "not-json"].join("\n"));
  assert.equal(events.sourcePullCommands, 1);
  assert.equal(events.sourceInspectCommands, 1);
  assert.equal(events.sourceFailedCommands, 1);
  assert.equal(events.sourceFailedPullCommands, 1);
  assert.equal(events.unparsedLines, 1);
  assert.deepEqual(classifyNegativeDevcontainerRefusal(events, 1), {
    phase: "prebuild-private-source-auth-refusal",
    composeBuildObserved: false,
  });
  for (const text of [
    [pull, auth].join("\n"),
    [pull, failure].join("\n"),
    // Local NoSuchImage plus an OCI auth failure does not prove native pull refusal.
    [
      inspect,
      event(
        "text",
        `Command failed: ${wrapper} inspect --type image ${state.image}`,
      ),
      auth,
    ].join("\n"),
    [inspect, auth, failure.replace(state.image, `${state.image}-other`)].join(
      "\n",
    ),
    [pull, failure, auth]
      .map((line) => {
        const record = JSON.parse(line);
        record.text = record.text.replaceAll(
          wrapper,
          resolve("other/docker.exe"),
        );
        return JSON.stringify(record);
      })
      .join("\n"),
    [pull, failure, auth]
      .join("\n")
      .replaceAll(state.image, `${state.image}-suffix`),
    [pull, failure, auth].join("\n") +
      "\n" +
      event(
        "start",
        `Run: ${wrapper} compose --project-name ${state.nonce} up -d`,
      ),
  ])
    assert.throws(() => classifyNegativeDevcontainerRefusal(parse(text), 1));
  assert.throws(() => classifyNegativeDevcontainerRefusal(events, 0));
  assert.throws(() => classifyNegativeDevcontainerRefusal(events, null));
  assert.equal(JSON.stringify(events).includes(wrapper), false);
  assert.equal(JSON.stringify(events).includes(state.image), false);
});

test("negative Compose invocation remains distinct from prebuild refusal and needs generated source separately", () => {
  const { state } = fixture(),
    wrapper = resolve("artifacts/package/orbwin-docker.exe");
  const output = [
    {
      type: "start",
      text: `Run: ${wrapper} compose --project-name ${state.nonce} build --no-cache app`,
    },
    { type: "raw", text: "unexpected status 401 Unauthorized" },
  ]
    .map(JSON.stringify)
    .join("\n");
  const events = negativeDevcontainerCommandEvidence(output, wrapper, state);
  assert.deepEqual(classifyNegativeDevcontainerRefusal(events, 1), {
    phase: "compose-build-auth-refusal",
    composeBuildObserved: true,
  });
  assert.throws(() => generatedDockerfilePath(output));
  assert.equal(events.generatedOverrideEvents, 0);
  assert.equal(events.sourcePullCommands, 0);
  assert.equal(events.defaultPlugin, undefined);
});

test("cumulative callback deadline clamps every command and reserves only bounded cleanup", () => {
  let now = 0;
  const budget = createCallbackBudget(180000, 20000, () => now);
  assert.equal(budget.limitTimeout(300000), 160000);
  now = 155000;
  assert.equal(budget.limitTimeout(120000), 5000);
  now = 160001;
  assert.throws(() => budget.assertCurrent(), {
    code: "DC_CALLBACK_DEADLINE_EXCEEDED",
  });
  budget.withCleanupBudget(() => {
    assert.equal(budget.phase(), "cleanup-reserve");
    assert.equal(budget.limitTimeout(30000), 19999);
    now = 180001;
    assert.throws(() => budget.assertCurrent(), {
      code: "DC_CALLBACK_DEADLINE_EXCEEDED",
    });
  });
  assert.equal(budget.phase(), "work");
  // A child that returns zero after its cumulative work deadline cannot pass.
  assert.throws(() => budget.assertCurrent(), {
    code: "DC_CALLBACK_DEADLINE_EXCEEDED",
  });
  const negative = createCallbackBudget(90000, 20000, () => 0);
  assert.equal(negative.limitTimeout(120000), 70000);
});

function fixture() {
  const nonce = "a".repeat(32),
    state = {
      nonce,
      name: `orbwin-private-client-${nonce}`,
      server: "127.0.0.1:54321",
      image: `127.0.0.1:54321/fixture/image:${nonce}`,
      workspace: resolve(tmpdir(), `orbwin-private-client-${nonce}`),
      committedRootFS: { Layers: [`sha256:${"b".repeat(64)}`] },
      before: { inventories: { owned: { images: [], containers: [] } } },
      intents: [],
      wrapper: "proof-only",
    };
  const plan = devcontainerComposePlan(state),
    image = {
      Id: `sha256:${"c".repeat(64)}`,
      RepoTags: [plan.tag],
      Config: {
        Labels: {
          "dev.orbwin.qualification": nonce,
          "dev.orbwin.private-devcontainer-compose": plan.marker,
        },
        Cmd: [
          "/bin/busybox",
          "timeout",
          "-s",
          "KILL",
          "300",
          "/bin/busybox",
          "sleep",
          "300",
        ],
      },
      RootFS: {
        Layers: [...state.committedRootFS.Layers, `sha256:${"d".repeat(64)}`],
      },
      Os: "linux",
      Architecture: "amd64",
    };
  const container = {
    Id: "e".repeat(64),
    Name: `/${plan.name}`,
    Image: image.Id,
    Config: {
      Image: plan.tag,
      Labels: { "dev.orbwin.qualification": nonce },
      Cmd: image.Config.Cmd,
      Entrypoint: [
        "/bin/sh",
        "-c",
        'echo Container started\ntrap "exit 0" 15\n\nexec "$@"\n\nwhile sleep 1 & wait $!; do :; done',
        "-",
      ],
    },
    HostConfig: {
      NetworkMode: "none",
      Privileged: false,
      ReadonlyRootfs: false,
      NanoCpus: 250000000,
      Memory: 134217728,
      PidsLimit: 64,
      CapDrop: ["ALL"],
      SecurityOpt: ["no-new-privileges:true"],
      RestartPolicy: { Name: "no" },
    },
    State: { Paused: false, Running: true },
    Mounts: [
      {
        Type: "bind",
        Destination: "/workspace",
        RW: true,
        Source: plan.workspace,
      },
    ],
  };
  return { state, plan, image, container };
}
test("private Compose source keeps generated feature stages but rejects foreign directives", () => {
  const { state, plan } = fixture();
  const expected = `ARG _DEV_CONTAINERS_BASE_IMAGE=scratch\n${plan.dockerfile.replace(`FROM ${state.image}\n`, `FROM ${state.image} AS dev_container_auto_added_stage_label\n`)}\nFROM $_DEV_CONTAINERS_BASE_IMAGE AS dev_containers_target_stage\nLABEL devcontainer.metadata="[]"\n`;
  verifyGeneratedDockerfile(expected, plan, state);
  for (const changed of [
    expected + "RUN echo external\n",
    expected + "run echo hidden\n",
    expected + "ENV TOKEN=unreviewed\n",
    expected.replace(state.image, "public:latest"),
    "# syntax=external/frontend\n" + expected,
    expected + "COPY private-secret /tmp/secret\n",
  ])
    assert.throws(() => verifyGeneratedDockerfile(changed, plan, state));
});
test("base removal refuses foreign identity, extra tags, baseline data and changed content", () => {
  const { state, plan } = fixture();
  state.committedId = `sha256:${"d".repeat(64)}`;
  const base = {
    Id: state.committedId,
    RepoTags: [state.image],
    Os: "linux",
    Architecture: "amd64",
    RootFS: state.committedRootFS,
    Config: {
      Labels: { "dev.orbwin.qualification": state.nonce },
      Cmd: plan.compose.services.app.command,
    },
  };
  assert.equal(ownedDevcontainerComposeBase(base, state), state.committedId);
  for (const mutate of [
    (x) => (x.Id = `sha256:${"e".repeat(64)}`),
    (x) => x.RepoTags.push("unrelated:keep"),
    (x) => (x.Config.Labels["dev.orbwin.qualification"] = "foreign"),
    (x) => (x.Config.Cmd = ["sleep", "forever"]),
    (x) => x.RootFS.Layers.push("foreign"),
    (x) => (x.Config.Volumes = { "/data": {} }),
  ]) {
    const changed = structuredClone(base);
    mutate(changed);
    assert.throws(() => ownedDevcontainerComposeBase(changed, state));
  }
  const baseline = structuredClone(state);
  baseline.before.inventories.owned.images.push(state.committedId);
  assert.throws(() => ownedDevcontainerComposeBase(base, baseline));
});
test("generated Dockerfile proof uses only the actual build override event", () => {
  const path = resolve(
    tmpdir(),
    "devcontainercli/container-features/owned/Dockerfile-with-features",
  );
  const event = JSON.stringify({
    type: "text",
    text: `Docker Compose override file for building image:\nservices:\n  app:\n    build:\n      dockerfile: ${path}\n`,
  });
  assert.equal(generatedDockerfilePath(event), path);
  assert.throws(() =>
    generatedDockerfilePath(JSON.stringify({ text: `unrelated ${path}` })),
  );
  assert.throws(() =>
    generatedDockerfilePath(event + "\n" + event.replace("owned", "another")),
  );
  assert.throws(() =>
    generatedDockerfilePath(
      event.replace("Dockerfile-with-features", "directory"),
    ),
  );
});
test("a version probe or standalone fallback does not prove original default-plugin build/up", () => {
  const nonce = "a".repeat(32),
    wrapper = resolve("artifacts/package/orbwin-docker.exe");
  const event = (text) => JSON.stringify({ type: "start", text });
  const build = event(
    `Run: ${wrapper} compose --project-name proof-${nonce} build --no-cache app`,
  );
  const up = event(
    `Run: ${wrapper} compose --project-name proof-${nonce} up -d app`,
  );
  assert.deepEqual(
    defaultComposeCommandEvidence(build + "\n" + up, wrapper, nonce),
    {
      defaultPlugin: true,
      buildCommands: 1,
      upCommands: 1,
      runtimeGrantArgvObserved: false,
    },
  );
  assert.throws(() =>
    defaultComposeCommandEvidence(
      event(`Run: ${wrapper} compose version --short`),
      wrapper,
      nonce,
    ),
  );
  assert.throws(() =>
    defaultComposeCommandEvidence(
      event(
        `Run: ${resolve("other/docker-compose.exe")} --project-name proof-${nonce} build --no-cache app`,
      ) +
        "\n" +
        up,
      wrapper,
      nonce,
    ),
  );
  assert.throws(() => defaultComposeCommandEvidence(build, wrapper, nonce));
  assert.equal(
    defaultComposeCommandEvidence(build, wrapper, nonce, true).buildCommands,
    1,
  );
});
test("cleanup ownership requires finite exec path, exact bind and immutable base marker", () => {
  const original = fixture();
  assert.equal(
    ownedDevcontainerComposeImage(
      original.image,
      original.state,
      original.plan,
    ),
    original.image.Id,
  );
  assert.equal(
    ownedDevcontainerComposeContainer(
      original.container,
      original.image,
      original.state,
      original.plan,
    ),
    original.container.Id,
  );
  for (const change of [
    ({ container }) => (container.Config.Cmd = []),
    ({ container }) =>
      (container.Config.Entrypoint[2] = "while sleep 1; do :; done"),
    ({ container }) =>
      (container.Mounts[0].Source = resolve(tmpdir(), "other-workspace")),
    ({ container }) =>
      container.Mounts.push({
        Type: "bind",
        Source: "/private/config",
        Destination: "/credentials",
      }),
    ({ container }) => (container.HostConfig.NetworkMode = "bridge"),
    ({ container }) => (container.HostConfig.Memory = 0),
    ({ state, container }) =>
      state.before.inventories.owned.containers.push(container.Id),
    ({ image }) =>
      (image.Config.Labels["dev.orbwin.private-devcontainer-compose"] =
        "replacement"),
    ({ image }) => image.RepoTags.push("user-image:keep"),
    ({ image }) => (image.RootFS.Layers[0] = `sha256:${"f".repeat(64)}`),
  ]) {
    const value = fixture();
    change(value);
    assert.throws(() =>
      ownedDevcontainerComposeContainer(
        value.container,
        value.image,
        value.state,
        value.plan,
      ),
    );
  }
});
test("guarded cleanup refuses an altered image at a known output tag before any deletion", () => {
  const { state, plan, image } = fixture();
  state.privateDevcontainerCompose = { imageIds: [], outputTags: [plan.tag] };
  image.Config.Labels["dev.orbwin.private-devcontainer-compose"] =
    "someone-else";
  const calls = [],
    ctx = {
      state,
      save() {},
      execute(_binary, args) {
        calls.push(args);
        if (args[0] === "ps") return { stdout: "", status: 0 };
        if (args[0] === "image" && args[1] === "ls")
          return { stdout: image.Id, status: 0 };
        if (args[0] === "image" && args[1] === "inspect")
          return { stdout: JSON.stringify([image]), status: 0 };
        throw Error("Unexpected command");
      },
    };
  assert.throws(() => cleanupPrivateDevcontainerCompose(ctx));
  assert.ok(
    calls.every(
      (args) =>
        !args.includes("rm") &&
        !args.includes("stop") &&
        !args.includes("prune"),
    ),
  );
});
test("Dev Containers context cannot bind parent auth workspace or opt into extras", () => {
  const { state, plan } = fixture();
  assert.notEqual(plan.workspace, state.workspace);
  assert.equal(plan.compose.services.app.volumes[0].source, plan.workspace);
  assert.equal(plan.config.features, undefined);
  assert.equal(plan.config.postCreateCommand, undefined);
  assert.equal(plan.compose.services.app.build.pull, true);
  assert.equal(plan.compose.services.app.build.no_cache, true);
  assert.equal(plan.compose.services.app.build.network, "none");
  assert.equal(plan.compose.services.app.ports, undefined);
  assert.throws(() =>
    devcontainerComposePlan({ ...state, image: "public:latest" }),
  );
});
