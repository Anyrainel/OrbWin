import test from "node:test";
import assert from "node:assert/strict";
import {
  append,
  composeFile,
  entries,
  imageUsers,
  labels,
  percent,
  ports,
  volumeUsers,
} from "../src/components/resources/model.ts";

test("published ports link only loopback/any TCP bindings, deduplicated", () => {
  const parsed = ports(
    "0.0.0.0:8080->80/tcp, [::]:8080->80/tcp, 192.168.1.5:9000->9000/tcp, 127.0.0.1:5353->53/udp, 6379/tcp",
  );
  assert.deepEqual(
    parsed.map((port) => [port.hostPort, port.protocol, port.url]),
    [
      [8080, "tcp", "http://localhost:8080/"],
      [9000, "tcp", undefined],
      [5353, "udp", undefined],
    ],
  );
  assert.deepEqual(ports(undefined), []);
  assert.deepEqual(ports("garbage"), []);
});

test("CLI label strings keep commas inside values and accept objects", () => {
  assert.deepEqual(
    labels({ Labels: "a=1,files=C:\\x\\a.yml,C:\\x\\b.yml,b=2" }),
    { a: "1", files: "C:\\x\\a.yml,C:\\x\\b.yml", b: "2" },
  );
  assert.deepEqual(labels({ Labels: { a: 1 } }), { a: "1" });
  assert.deepEqual(labels({}), {});
});

test("Compose file is known only for one recorded Windows path", () => {
  const row = (files) => ({
    Labels: `com.docker.compose.project=p,com.docker.compose.project.config_files=${files}`,
  });
  assert.equal(
    composeFile([row("C:\\app\\compose.yml")]),
    "C:\\app\\compose.yml",
  );
  assert.equal(composeFile([row("/home/u/compose.yml")]), undefined);
  assert.equal(composeFile([row("C:\\app\\a.yml,C:\\app\\b.yml")]), undefined);
  assert.equal(
    composeFile([row("C:\\a\\compose.yml"), row("C:\\b\\compose.yml")]),
    undefined,
  );
  assert.equal(
    composeFile([{ Labels: "com.docker.compose.project=p" }]),
    undefined,
  );
});

test("entries group by returned project label and sort running first", () => {
  const rows = [
    { Names: "b", State: "exited" },
    { Names: "a-db", State: "exited", Labels: "com.docker.compose.project=a" },
    { Names: "c", State: "running" },
    {
      Names: "a-web",
      State: "running",
      Labels: "com.docker.compose.project=a",
    },
  ];
  const status = entries("containers", rows, "status");
  assert.deepEqual(
    status.map((e) => (e.kind === "row" ? e.row.Names : `[${e.project}]`)),
    ["[a]", "c", "b"],
  );
  assert.deepEqual(
    status[0].rows.map((row) => row.Names),
    ["a-web", "a-db"],
  );
  const byName = entries("containers", rows, "name");
  assert.deepEqual(
    byName.map((e) => (e.kind === "row" ? e.row.Names : `[${e.project}]`)),
    ["[a]", "b", "c"],
  );
  // Labels on non-container pages never form groups.
  assert.ok(
    entries(
      "volumes",
      [{ Name: "v", Labels: "com.docker.compose.project=a" }],
      "status",
    ).every((e) => e.kind === "row"),
  );
});

test("usage matches exact returned references only", () => {
  const containers = [
    { Names: "one", Image: "redis:alpine", Mounts: "data,/mnt/f/src" },
    { Names: "two", Image: "nginx", Mounts: "database" },
    { Names: "three", Image: "sha256:abc" },
  ];
  assert.deepEqual(
    volumeUsers("data", containers).map((r) => r.Names),
    ["one"],
  );
  assert.deepEqual(
    imageUsers(
      { Repository: "nginx", Tag: "latest", ID: "sha256:x" },
      containers,
    ).map((r) => r.Names),
    ["two"],
  );
  assert.deepEqual(
    imageUsers(
      { Repository: "<none>", Tag: "<none>", ID: "sha256:abc" },
      containers,
    ).map((r) => r.Names),
    ["three"],
  );
  assert.deepEqual(
    imageUsers({ Repository: "redis", Tag: "7", ID: "sha256:y" }, containers),
    [],
  );
});

test("stats parsing and bounded history", () => {
  assert.equal(percent("12.5%"), 12.5);
  assert.equal(percent("--"), undefined);
  assert.equal(percent(undefined), undefined);
  let history = [];
  for (let i = 0; i < 70; i++) history = append(history, i, 60);
  assert.equal(history.length, 60);
  assert.equal(history[0], 10);
  assert.equal(history.at(-1), 69);
});
