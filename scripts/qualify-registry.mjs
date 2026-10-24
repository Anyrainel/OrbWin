// Explicit local authentication fixture. The password is intentionally public,
// disposable test data; no user credentials are requested or saved in evidence.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { verifyBuild } from "./build-identity.mjs";

assert.equal(process.env.ORBWIN_QUALIFY_OWNED, "1");
const identity = JSON.parse(
  readFileSync("artifacts/qualification/engine-identity.json"),
);
assert.equal(identity.coexistenceQualified, true);
assert.equal(identity.binarySha256, verifyBuild(process.cwd()).binarySha256);
const docker = resolve("artifacts/runtime-tools/bin/docker.exe");
const helper = resolve("artifacts/backend/docker-credential-orbwin.exe");
const helperManifest = JSON.parse(
  readFileSync("artifacts/backend/credentials-manifest.json"),
);
assert.equal(
  createHash("sha256").update(readFileSync(helper)).digest("hex"),
  helperManifest.binarySha256,
);
const config = resolve(identity.home, ".rdorbwin/docker");
const env = {
  ...process.env,
  PATH: resolve("artifacts/backend") + ";" + process.env.PATH,
  DOCKER_CONTEXT: "",
  DOCKER_TLS_VERIFY: "",
  DOCKER_CERT_PATH: "",
  BUILDKIT_HOST: "",
  DOCKER_CONFIG: config,
  DOCKER_HOST: identity.endpoint,
};
function cli(args, input, seconds = 90) {
  return execFileSync(
    docker,
    ["--config", config, "--host", identity.endpoint, ...args],
    {
      env,
      input,
      encoding: "utf8",
      timeout: seconds * 1000,
      maxBuffer: 2 << 20,
      windowsHide: true,
    },
  ).trim();
}
assert.equal(
  cli(["info", "--format", "{{.ID}} {{.OSType}}"]),
  `${identity.daemonId} linux`,
);
const pins = JSON.parse(readFileSync("tests/registry/images.lock.json"));
const password = "orbwin-disposable-proof-only";
const mode = process.argv[2];
let state, file;
if (mode === "prepare") {
  const run = randomUUID(),
    name = `orbwin-registry-${run.slice(0, 8)}`;
  const directory = resolve("artifacts/qualification", name);
  mkdirSync(directory);
  const hash = execFileSync(
    resolve("artifacts/qualification/registry-fixture-hash.exe"),
    [],
    { input: password, encoding: "utf8", windowsHide: true, timeout: 10000 },
  ).trim();
  const auth = resolve(directory, "htpasswd");
  writeFileSync(auth, `orbwin-proof:${hash}\n`);
  const volume = cli([
    "volume",
    "create",
    "--label",
    `org.orbwin.qualification=${run}`,
    name,
  ]);
  const container = cli([
    "run",
    "--detach",
    "--name",
    name,
    "--label",
    `org.orbwin.qualification=${run}`,
    "--publish",
    "127.0.0.1::5000",
    "--mount",
    `type=bind,source=${auth},target=/auth/htpasswd,readonly`,
    "--mount",
    `type=volume,source=${volume},target=/var/lib/registry`,
    "--env",
    "REGISTRY_AUTH=htpasswd",
    "--env",
    "REGISTRY_AUTH_HTPASSWD_REALM=OrbWin qualification",
    "--env",
    "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd",
    pins.registry,
  ]);
  const inspected = JSON.parse(cli(["inspect", container]))[0];
  const port = Number(inspected.NetworkSettings.Ports["5000/tcp"][0].HostPort);
  state = {
    run,
    name,
    directory,
    container,
    volume,
    server: `127.0.0.1:${port}`,
    username: "orbwin-proof",
    identity,
    helperSha256: helperManifest.binarySha256,
    pins,
    globalConfigs: {},
    checks: [],
  };
  for (const path of [
    resolve(process.env.USERPROFILE, ".docker/config.json"),
    resolve(process.env.USERPROFILE, ".kube/config"),
  ])
    if (existsSync(path))
      state.globalConfigs[path] = createHash("sha256")
        .update(readFileSync(path))
        .digest("hex");
  state.image = `${state.server}/orbwin-proof/image:${run.slice(0, 8)}`;
  state.builtImage = `orbwin-registry-build:${run.slice(0, 8)}`;
  file = resolve(directory, "state.json");
  writeFileSync(file, JSON.stringify(state, null, 2));
  const deadline = Date.now() + 20000;
  for (;;) {
    try {
      const response = await fetch(`http://${state.server}/v2/`, {
        signal: AbortSignal.timeout(5000),
      });
      assert.equal(response.status, 401);
      break;
    } catch (error) {
      if (Date.now() >= deadline || error.code === "ERR_ASSERTION") throw error;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  }
  state.checks.push("unauthenticated registry request rejected with HTTP 401");
} else {
  assert.match(process.argv[3] ?? "", /^orbwin-registry-[a-f0-9]{8}$/);
  file = resolve("artifacts/qualification", process.argv[3], "state.json");
  state = JSON.parse(readFileSync(file));
  assert.equal(state.identity.daemonId, identity.daemonId);
  assert.equal(state.identity.binarySha256, identity.binarySha256);
  assert.equal(state.helperSha256, helperManifest.binarySha256);
  const inspected = JSON.parse(cli(["inspect", state.container]))[0];
  assert.equal(inspected.Id, state.container);
  assert.equal(inspected.Config.Labels["org.orbwin.qualification"], state.run);
  if (mode === "push") {
    const listed = JSON.parse(
      execFileSync(helper, ["list"], {
        encoding: "utf8",
        windowsHide: true,
        timeout: 10000,
      }),
    );
    assert.equal(
      listed[state.server],
      state.username,
      "Sign in through the native app first",
    );
    cli(["image", "tag", pins.base, state.image]);
    const pushOutput = cli(["image", "push", state.image], undefined, 120);
    state.registryManifestDigest = pushOutput.match(
      /digest:\s*(sha256:[a-f0-9]{64})/,
    )?.[1];
    assert(
      state.registryManifestDigest,
      "Registry push did not report a content digest",
    );
    state.localImageIdBeforePull = JSON.parse(
      cli(["image", "inspect", state.image]),
    )[0].Id;
    cli(["image", "rm", state.image]);
    state.checks.push(
      "private push used the native login's product helper; local registry tag removed for native pull",
    );
  } else if (mode === "verify-pull") {
    const manifestResponse = await fetch(
      `http://${state.server}/v2/orbwin-proof/image/manifests/${state.run.slice(0, 8)}`,
      {
        headers: {
          Authorization: `Basic ${Buffer.from(`${state.username}:${password}`).toString("base64")}`,
          Accept:
            "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json",
        },
        signal: AbortSignal.timeout(10000),
      },
    );
    assert.equal(manifestResponse.status, 200);
    const manifestDigest = manifestResponse.headers.get(
      "docker-content-digest",
    );
    assert.match(manifestDigest ?? "", /^sha256:[a-f0-9]{64}$/);
    if (state.registryManifestDigest)
      assert.equal(manifestDigest, state.registryManifestDigest);
    else state.registryManifestDigest = manifestDigest;
    const image = JSON.parse(cli(["image", "inspect", state.image]))[0];
    assert(
      image.RepoDigests.includes(
        `${state.server}/orbwin-proof/image@${manifestDigest}`,
      ),
      "Native pull must match the independently read registry manifest",
    );
    if (state.digest)
      state.initialImageIdExpectation = {
        value: state.digest,
        correction:
          "Docker containerd image-store index ID before push differs from the pushed platform manifest; verify against the registry's content digest",
      };
    const configuration = JSON.parse(
      readFileSync(resolve(config, "config.json")),
    );
    assert.equal(configuration.credHelpers[state.server], "orbwin");
    assert(
      !configuration.auths?.[state.server]?.auth,
      "Password must not be in the Docker config file",
    );
    state.checks.push(
      "native authenticated pull restored the exact image without file credentials",
    );
  } else if (mode === "wsl-pull") {
    const linux = (path) =>
      `/mnt/${path[0].toLowerCase()}/${path.slice(3).replaceAll("\\", "/")}`;
    const wslConfig = resolve(identity.home, ".rdorbwin/wsl-docker");
    assert.equal(
      JSON.parse(readFileSync(resolve(wslConfig, "config.json"))).credHelpers[
        state.server
      ],
      "orbwin.exe",
    );
    const wsl = resolve(process.env.SystemRoot, "System32/wsl.exe");
    const shell = (script, args = []) =>
      execFileSync(
        wsl,
        ["-d", "Ubuntu", "--exec", "sh", "-c", script, "orbwin", ...args],
        {
          encoding: "utf8",
          windowsHide: true,
          timeout: 120000,
          maxBuffer: 2 << 20,
        },
      ).trim();
    assert.notEqual(shell("id -u"), "0", "Qualify the ordinary WSL user");
    const globalBefore = shell(
      'if test -f "$HOME/.docker/config.json"; then sha256sum "$HOME/.docker/config.json"; else printf absent; fi',
    );
    const dockerLinux = linux(resolve("artifacts/runtime-tools/linux/docker"));
    const args = [
      linux(resolve("artifacts/backend")),
      dockerLinux,
      "--config",
      linux(wslConfig),
      "--host",
      "unix:///mnt/wsl/orbwin/docker.sock",
    ];
    const run = (...command) =>
      shell(
        'export PATH="$1:$PATH"; shift; exec env -u DOCKER_CONTEXT -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH -u BUILDKIT_HOST BUILDX_BUILDER=default "$@"',
        [...args, ...command],
      );
    assert.equal(run("info", "--format", "{{.ID}}"), identity.daemonId);
    run("pull", state.image);
    const pulled = JSON.parse(run("image", "inspect", state.image))[0];
    assert(
      pulled.RepoDigests.some((value) =>
        value.endsWith(`@${state.registryManifestDigest}`),
      ),
    );
    assert.equal(
      shell(
        'if test -f "$HOME/.docker/config.json"; then sha256sum "$HOME/.docker/config.json"; else printf absent; fi',
      ),
      globalBefore,
    );
    state.checks.push(
      "ordinary Ubuntu user authenticated pull through Windows vault interop; exact manifest and global WSL config preserved",
    );
  } else if (mode === "prepare-build") {
    state.buildDirectory = resolve(state.directory, "build");
    mkdirSync(state.buildDirectory, { recursive: true });
    writeFileSync(
      resolve(state.buildDirectory, "Dockerfile"),
      `FROM ${state.image}\nLABEL org.orbwin.qualification="${state.run}"\nRUN printf '%s' 'orbwin-registry-build-${state.run}' > /orbwin-registry-proof\n`,
    );
    if (cli(["image", "ls", "--filter", `reference=${state.image}`, "--quiet"]))
      cli(["image", "rm", state.image]);
    state.checks.push(
      "private base tag removed before native Buildx authentication check",
    );
  } else if (mode === "verify-build") {
    const image = JSON.parse(cli(["image", "inspect", state.builtImage]))[0];
    assert.equal(image.Config.Labels["org.orbwin.qualification"], state.run);
    assert.equal(
      cli([
        "run",
        "--rm",
        "--name",
        `${state.name}-build-check`,
        "--label",
        `org.orbwin.qualification=${state.run}`,
        state.builtImage,
        "cat",
        "/orbwin-registry-proof",
      ]),
      `orbwin-registry-build-${state.run}`,
    );
    state.checks.push(
      "native private-base Buildx result executes and contains its unique build marker",
    );
  } else if (mode === "verify-forget") {
    const saved = JSON.parse(
      execFileSync(helper, ["list"], {
        encoding: "utf8",
        windowsHide: true,
        timeout: 10000,
      }),
    );
    assert.equal(
      saved[state.server],
      undefined,
      "Forget through the native application first",
    );
    if (cli(["image", "ls", "--filter", `reference=${state.image}`, "--quiet"]))
      cli(["image", "rm", state.image]);
    let rejected = false;
    try {
      cli(["pull", state.image]);
    } catch (error) {
      assert.match(
        String(error.stderr),
        /unauthorized|authentication required|no basic auth credentials/,
      );
      rejected = true;
    }
    assert(
      rejected,
      "A private pull must fail after forgetting the credential",
    );
    assert.equal(
      JSON.parse(cli(["inspect", state.container]))[0].State.Running,
      true,
    );
    state.checks.push(
      "native Forget removed only the selected credential; subsequent private pull refused and registry workload retained",
    );
  } else if (mode === "pause" || mode === "resume") {
    cli([mode === "pause" ? "pause" : "unpause", state.container]);
    state.checks.push(`${mode} only the scoped registry fixture`);
  } else if (mode === "remove-tag") {
    cli(["image", "rm", state.image]);
  } else if (mode === "cleanup") {
    if (inspected.State.Paused) cli(["unpause", state.container]);
    cli(["stop", state.container]);
    cli(["rm", state.container]);
    const volume = JSON.parse(cli(["volume", "inspect", state.volume]))[0];
    assert.equal(volume.Labels["org.orbwin.qualification"], state.run);
    cli(["volume", "rm", state.volume]);
    execFileSync(helper, ["erase"], {
      input: state.server,
      windowsHide: true,
      timeout: 10000,
    });
    for (const image of [state.image, state.builtImage]) {
      const tagged = cli([
        "image",
        "ls",
        "--filter",
        `reference=${image}`,
        "--quiet",
        "--no-trunc",
      ]);
      if (tagged) cli(["image", "rm", image]);
    }
    state.cleaned = true;
    state.checks.push(
      "exact scoped registry container, volume and credential cleaned",
    );
  } else throw new Error("Unknown qualification action");
}
for (const [path, hash] of Object.entries(state.globalConfigs))
  assert.equal(
    createHash("sha256").update(readFileSync(path)).digest("hex"),
    hash,
    "A global client config changed",
  );
writeFileSync(file, JSON.stringify(state, null, 2));
console.log(
  `Registry fixture: ${state.name}; server ${state.server}; image ${state.image}`,
);
