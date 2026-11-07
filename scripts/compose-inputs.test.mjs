import test from "node:test";
import assert from "node:assert/strict";
import {
  composeInputs,
  composeServiceInputs,
  wslComposeInputs,
} from "../src/components/resources/model.ts";
import {
  loadComposeInputs,
  rememberComposeInputs,
  forgetComposeInputs,
} from "../src/model/compose-inputs.ts";

const row = (
  files = "C:\\project\\base.yml,C:\\project\\override.yml",
  directory = "C:\\project",
) => ({
  Labels: {
    "com.docker.compose.project": "dev",
    "com.docker.compose.project.config_files": files,
    "com.docker.compose.project.working_dir": directory,
  },
});
test("discovery preserves ordered files and explicit directory without guessing profiles", () => {
  const expected = {
    project: "dev",
    file: "C:\\project\\base.yml",
    overrides: ["C:\\project\\override.yml"],
    profiles: [],
    directory: "C:\\project",
  };
  assert.deepEqual(composeInputs("dev", [row(), row()]), expected);
  const formatted = row();
  formatted.Labels = Object.entries(formatted.Labels)
    .map(([k, v]) => `${k}=${v}`)
    .join(",");
  assert.deepEqual(composeInputs("dev", [formatted]), expected);
  assert.deepEqual(
    composeInputs("dev", [
      row(
        "\\\\?\\C:\\project\\base.yml,\\\\?\\C:\\project\\override.yml",
        "\\\\?\\C:\\project",
      ),
    ]),
    expected,
  );
});
test("WSL discovery reuses only coherent bounded original project inputs", () => {
  const inputs = {
    distribution: "Ubuntu",
    source: "/home/user/project ü",
    file: "compose.yaml",
    overrides: ["extra.yaml"],
    directory: "",
    project: "dev",
    profiles: ["worker"],
  };
  const member = {
    Labels: {
      "com.docker.compose.project": "dev",
      "dev.orbwin.wsl.compose-inputs": JSON.stringify(inputs),
    },
  };
  assert.deepEqual(wslComposeInputs("dev", [member, member]), inputs);
  const cli = {
    Labels: Object.entries(member.Labels)
      .map(([key, value]) => `${key}=${value}`)
      .join(","),
  };
  assert.deepEqual(wslComposeInputs("dev", [cli]), inputs);
  for (const rows of [
    [],
    [member, row()],
    [
      member,
      {
        Labels: {
          ...member.Labels,
          "dev.orbwin.wsl.compose-inputs": JSON.stringify({
            ...inputs,
            distribution: "Other",
          }),
        },
      },
    ],
    [
      {
        Labels: {
          ...member.Labels,
          "dev.orbwin.wsl.compose-inputs": "not-json",
        },
      },
    ],
    [
      {
        Labels: {
          ...member.Labels,
          "dev.orbwin.wsl.compose-inputs": JSON.stringify({
            ...inputs,
            source: "/etc",
          }),
        },
      },
    ],
  ])
    assert.equal(wslComposeInputs("dev", rows), undefined);
  assert.equal(wslComposeInputs("other", [member]), undefined);
});

test("reviewed env-file paths preserve order without storing parsed values", () => {
  const store = new Map();
  globalThis.localStorage = {
    getItem: (key) => store.get(key) ?? null,
    setItem: (key, value) => store.set(key, value),
  };
  const input = {
    project: "env-proof",
    file: "C:\\project\\compose.yaml",
    overrides: [],
    profiles: [],
    directory: "C:\\project",
    envFiles: ["C:\\project\\first.env", "C:\\project\\last 例.env"],
  };
  rememberComposeInputs(input);
  assert.deepEqual(loadComposeInputs()[0], input);
  assert.throws(() =>
    rememberComposeInputs({
      ...input,
      envFiles: Array(17).fill(input.envFiles[0]),
    }),
  );
  assert.throws(() =>
    rememberComposeInputs({ ...input, envFiles: ["bad\npath"] }),
  );
  const wsl = {
    distribution: "Ubuntu",
    source: "/home/user/project",
    file: "compose.yaml",
    overrides: [],
    directory: "",
    project: "env-proof",
    profiles: [],
    envFiles: ["first.env", "last 例.env"],
  };
  const member = (value) => ({
    Labels: {
      "com.docker.compose.project": "env-proof",
      "dev.orbwin.wsl.compose-inputs": JSON.stringify(value),
    },
  });
  assert.deepEqual(
    wslComposeInputs("env-proof", [member(wsl)]).envFiles,
    wsl.envFiles,
  );
  for (const envFiles of [
    Array(17).fill("first.env"),
    ["bad\nfile"],
    ["C:\\outside.env"],
  ])
    assert.equal(
      wslComposeInputs("env-proof", [member({ ...wsl, envFiles })]),
      undefined,
    );
});
test("discovery fails closed for conflicting, stale, unsupported or ambiguous metadata", () => {
  for (const rows of [
    [],
    [row(), row("C:\\other.yml")],
    [row(), row(undefined, "C:\\other")],
    [row("/home/dev/compose.yml")],
    [row("C:\\project\\base.yml,relative.yml")],
    [row("C:\\base\n.yml")],
    [row("C:\\base=bad.yml")],
    [row("\\\\server\\share\\compose.yml")],
    [row("\\\\?\\UNC\\server\\share\\compose.yml")],
    [row(Array(17).fill("C:\\a.yml").join(","))],
    [row(), { Labels: { "com.docker.compose.project": "dev" } }],
    [row(undefined, "/home/dev")],
    [
      {
        Labels: {
          ...row().Labels,
          "com.docker.compose.project.environment_file": "C:\\secret.env",
        },
      },
    ],
  ])
    assert.equal(composeInputs("dev", rows), undefined);
  assert.equal(composeInputs("other", [row()]), undefined);
  assert.equal(
    composeInputs("dev", [row("C:/project/compose.yml", "")]),
    undefined,
  );
  assert.equal(
    composeInputs("dev", [row("C:/project/compose.yml", "C:/project")]).file,
    "C:/project/compose.yml",
  );
});
test("remembered inputs survive reload, replace by project, forget without engine effects and reject corrupt storage", () => {
  const old = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  let raw = null;
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: () => raw,
      setItem: (_, value) => {
        raw = value;
      },
    },
  });
  try {
    const inputs = composeInputs("dev", [row()]);
    rememberComposeInputs(inputs);
    assert.deepEqual(loadComposeInputs(), [inputs]);
    rememberComposeInputs({ ...inputs, profiles: ["tools"] });
    assert.deepEqual(loadComposeInputs(), [{ ...inputs, profiles: ["tools"] }]);
    rememberComposeInputs({ ...inputs, services: ["worker"] });
    assert.deepEqual(loadComposeInputs()[0].services, ["worker"]);
    for (let i = 0; i < 25; i++)
      rememberComposeInputs({ ...inputs, project: `p${i}` });
    assert.equal(loadComposeInputs().length, 20);
    forgetComposeInputs("p24");
    assert.equal(
      loadComposeInputs().some((item) => item.project === "p24"),
      false,
    );
    for (const value of [
      "null",
      "{}",
      "broken",
      '[{"project":"dev"}]',
      "[]".repeat(1_000_001),
    ]) {
      raw = value;
      assert.deepEqual(loadComposeInputs(), []);
    }
    assert.throws(() =>
      rememberComposeInputs({ ...inputs, overrides: ["a\nb"] }),
    );
    // Corrupt records now fail closed before any write; isolate write denial.
    raw = "[]";
    localStorage.setItem = () => {
      throw new Error("Storage denied");
    };
    assert.throws(() => rememberComposeInputs(inputs), /Storage denied/);
  } finally {
    if (old) Object.defineProperty(globalThis, "localStorage", old);
    else delete globalThis.localStorage;
  }
});
test("service shortcuts use exact labels, never container names or one-off membership", () => {
  const container = { Names: "dev-wrong-1", ...row() };
  assert.equal(composeServiceInputs(container), undefined);
  container.Labels["com.docker.compose.service"] = "worker";
  assert.deepEqual(composeServiceInputs(container).services, ["worker"]);
  container.Labels["com.docker.compose.oneoff"] = "True";
  assert.equal(composeServiceInputs(container), undefined);
});
