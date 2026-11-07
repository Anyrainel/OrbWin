import test from "node:test";
import assert from "node:assert/strict";
import { forwardEndpoint } from "../src/components/cluster/forward-endpoint.ts";
import { forwardUrls } from "../src/components/cluster/model.ts";
import { buildSync } from "esbuild";
import { createRequire } from "node:module";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { fileURLToPath } from "node:url";

test("TCP forwards have an address without an assumed web protocol", () => {
  assert.deepEqual(forwardEndpoint(54321, 5432, "tcp"), {
    address: "127.0.0.1:54321",
    url: null,
  });
  assert.deepEqual(forwardEndpoint(1, 65535, "https"), {
    address: "127.0.0.1:1",
    url: "https://127.0.0.1:1/",
  });
  assert.deepEqual(forwardEndpoint(65535, 1, "http"), {
    address: "127.0.0.1:65535",
    url: "http://127.0.0.1:65535/",
  });
});

test("forward panel initially offers explicit protocol choice without a web link", () => {
  const { outputFiles } = buildSync({
    entryPoints: [
      fileURLToPath(
        new URL(
          "../src/components/cluster/ForwardEndpoint.tsx",
          import.meta.url,
        ),
      ),
    ],
    bundle: true,
    write: false,
    platform: "node",
    format: "cjs",
    external: ["react"],
    logLevel: "silent",
  });
  const module = { exports: {} };
  new Function("require", "module", "exports", outputFiles[0].text)(
    createRequire(import.meta.url),
    module,
    module.exports,
  );
  const html = renderToStaticMarkup(
    createElement(module.exports.ForwardEndpoint, {
      local: 54321,
      remote: 5432,
    }),
  );
  assert.match(html, /127\.0\.0\.1:54321/);
  assert.match(html, /Copy address/);
  assert.match(html, /TCP address only/);
  assert.match(html, />HTTP<\/option>/);
  assert.match(html, />HTTPS<\/option>/);
  assert.doesNotMatch(html, /<a\b|href=/);
});

test("endpoint inputs cannot select hosts, schemes, paths, queries or userinfo", () => {
  for (const value of [
    0,
    -1,
    65536,
    NaN,
    Infinity,
    80.5,
    "443",
    "443@foreign",
    "443/?q=x",
  ]) {
    assert.equal(forwardEndpoint(value, 443, "https"), null);
    assert.equal(forwardEndpoint(443, value, "https"), null);
  }
  for (const protocol of [
    "",
    "HTTP",
    "file",
    "javascript",
    "https://foreign",
    "https/other",
    "https?x=1",
    "https@foreign",
    "https\0",
  ]) {
    assert.equal(forwardEndpoint(443, 443, protocol), null);
  }
});

test("readiness accepts only complete valid loopback records and deduplicates", () => {
  assert.deepEqual(
    forwardUrls(
      "Forwarding from 127.0.0.1:61898 -> 80\r\nForwarding from 127.0.0.1:61898 -> 80\nHandling connection for 61898\n",
    ),
    [{ local: 61898, remote: 80 }],
  );
  for (const line of [
    "Forwarding from 127.0.0.1:0 -> 80",
    "Forwarding from 127.0.0.1:65536 -> 80",
    "Forwarding from 127.0.0.1:80 -> 0",
    "Forwarding from 127.0.0.1:80 -> 65536",
    "Forwarding from 0.0.0.0:80 -> 80",
    "Forwarding from [::1]:80 -> 80",
    "error: Forwarding from 127.0.0.1:80 -> 80",
    "Forwarding from 127.0.0.1:80 -> 80evil",
    "Forwarding from 127.0.0.1:80 -> 80/?foreign",
    "Forwarding from 127.0.0.1:123456 -> 80",
  ])
    assert.deepEqual(forwardUrls(line), [], line);
});

test("conflicting records for one local port never produce a misleading endpoint", () => {
  assert.deepEqual(
    forwardUrls(
      "Forwarding from 127.0.0.1:5000 -> 80\nForwarding from 127.0.0.1:5000 -> 443\nForwarding from 127.0.0.1:5000 -> 80\nForwarding from 127.0.0.1:6000 -> 5432",
    ),
    [{ local: 6000, remote: 5432 }],
  );
});
