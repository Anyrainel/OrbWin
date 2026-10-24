import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
const base = new URL("../.upstream/rancher-desktop/rdd/", import.meta.url);
const source = (path) => readFileSync(new URL(path, base), "utf8");
function value(text, name) {
  const match = text.match(
    new RegExp(`\\b${name}\\s+(?:uint32\\s+)?=\\s+(?:"([^"]+)"|(\\d+))`),
  );
  assert.ok(match, `Missing upstream contract constant: ${name}`);
  return match[1] ?? Number(match[2]);
}
test("host and rebuilt guest agree on a product-specific discovery and API namespace", () => {
  const host = source("pkg/hostswitch/hostswitch_windows.go");
  const guest = source("src/networking/pkg/vsock/constants.go");
  const bridge = source("pkg/socketbridge/host_windows.go");
  const api = source("src/rdd-guest/main.go");
  assert.equal(value(host, "signaturePhrase"), value(guest, "SignaturePhrase"));
  assert.notEqual(
    value(host, "signaturePhrase"),
    "github.com/rancher-sandbox/rancher-desktop-opensuse/src/go/networking",
  );
  assert.equal(
    value(host, "vsockHandshakePort"),
    value(guest, "DefaultHandshakePort"),
  );
  assert.equal(
    value(host, "vsockListenPort"),
    value(guest, "DefaultListenPort"),
  );
  assert.equal(
    value(bridge, "VsockForwardPort"),
    value(api, "dockerVsockPort"),
  );
  assert.equal(
    value(bridge, "ContainerdVsockPort"),
    value(api, "containerdVsockPort"),
  );
  const ports = [
    value(host, "vsockHandshakePort"),
    value(host, "vsockListenPort"),
    value(bridge, "VsockForwardPort"),
    value(bridge, "ContainerdVsockPort"),
  ];
  assert.equal(new Set(ports).size, ports.length);
  for (const port of ports)
    assert.ok(
      port > 0 && port < 65536 && ![6657, 6660, 6661, 6670].includes(port),
    );
});
test("WSL cleanup link names and bridge route are separated consistently", () => {
  const network = source("src/networking/cmd/network/setup_linux.go");
  const proxy = source("src/networking/cmd/proxy/wsl_integration_linux.go");
  assert.equal(value(network, "namespaceVethIP"), value(proxy, "bridgeIPAddr"));
  assert.notEqual(value(network, "WSLVethIP"), "192.168.143.2");
  for (const [name, original] of [
    ["WSLVeth", "veth-rd-wsl"],
    ["namespaceVeth", "veth-rd-ns"],
  ]) {
    const link = value(network, name);
    assert.notEqual(link, original);
    assert.ok(link.length <= 15, "Linux interface name exceeds IFNAMSIZ");
  }
});

test("the app selects a distinct name for Lima's globally registered WSL distro", () => {
  const app = source("pkg/controllers/app/app/controllers/app_controller.go");
  const match = app.match(/limaVMName,\s*inputConfigMapName\s*=\s*"([^"]+)"/);
  assert.ok(
    match,
    "App VM naming contract changed; review Lima WSL registration",
  );
  assert.equal(match[1], "orbwin");
  assert.notEqual(`lima-${match[1]}`, "lima-rd");
});
