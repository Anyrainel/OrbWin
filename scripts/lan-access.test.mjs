import test from "node:test";
import assert from "node:assert/strict";
import {
  lanClientNetworks,
  lanOptionsCurrent,
  lanRetainChoices,
  lanPublicationKey,
  lanParsePending,
  lanObservePending,
  lanReconcilePending,
  lanIngressFresh,
  lanReviewCurrent,
  lanReviewOperation,
} from "../src/model/lan-access.ts";

test("LAN client ranges retain exact private network boundaries", () => {
  assert.deepEqual(
    lanClientNetworks("192.168.1.0/24, 10.0.0.0/8\n172.16.0.0/12"),
    ["10.0.0.0/8", "172.16.0.0/12", "192.168.1.0/24"],
  );
  for (const value of [
    "",
    "0.0.0.0/0",
    "10.0.0.0/7",
    "172.0.0.0/8",
    "172.32.0.0/16",
    "192.168.1.10/24",
    "192.168.001.0/24",
    "192.168.1.0/024",
    "::1/128",
    "169.254.0.0/16",
    "127.0.0.1/32",
    "192.168.1.0/33",
    "10.0.0.0/8,10.0.0.0/8",
    Array.from({ length: 9 }, (_, i) => `10.${i}.0.0/16`).join(","),
  ]) {
    assert.throws(() => lanClientNetworks(value), undefined, value);
  }
});

test("ready LAN indication expires independently of a stalled snapshot", () => {
  const now = Date.parse("2026-10-26T05:00:00Z");
  const updatedAt = new Date(now - 1000).toISOString();
  const status = {
    operationId: "a".repeat(32),
    leaseSha256: "b".repeat(64),
    sequence: 1,
    phase: "ready",
    ingress: "ready",
    firewall: "created",
    updatedAt,
  };
  assert.equal(lanIngressFresh(status, updatedAt, true, now), true);
  assert.equal(lanIngressFresh(status, updatedAt, true, now + 4000), false);
  assert.equal(lanIngressFresh(status, updatedAt, false, now), false);
  assert.equal(lanIngressFresh(status, "invalid", true, now), false);
  assert.equal(
    lanIngressFresh(status, new Date(now + 1).toISOString(), true, now),
    false,
  );
  for (const change of [
    { ingress: "unknown" },
    { phase: "closing" },
    { firewall: "create-unknown" },
    { leaseSha256: undefined },
    { operationId: "foreign" },
    { sequence: Number.MAX_SAFE_INTEGER + 1 },
  ]) {
    assert.equal(
      lanIngressFresh({ ...status, ...change }, updatedAt, true, now),
      false,
    );
  }
});

test("LAN review confirmation refuses expiry and malformed owner references", () => {
  const now = Date.parse("2026-10-26T05:00:00Z");
  const review = {
    purpose: "activate",
    available: true,
    reviewId: "a".repeat(32),
    reviewSha256: "b".repeat(64),
    expiresAt: new Date(now + 60_000).toISOString(),
  };
  assert.equal(lanReviewCurrent(review, now), true);
  for (const change of [
    { available: false },
    { purpose: "unknown" },
    { reviewId: "a" },
    { reviewSha256: "B".repeat(64) },
    { expiresAt: new Date(now).toISOString() },
    { expiresAt: new Date(now + 60_001).toISOString() },
    { expiresAt: "invalid" },
  ]) {
    assert.equal(lanReviewCurrent({ ...review, ...change }, now), false);
  }
});

const instant = Date.parse("2026-10-26T05:00:00Z");
test("durable issuance revocation clears only an exact fresh unconsumed review", () => {
  const record = {
    runtimeInstance: "a".repeat(32),
    reviewId: "b".repeat(32),
    reviewSha256: "c".repeat(64),
    purpose: "activate",
  };
  const proof = {
    reviewId: record.reviewId,
    reviewSha256: record.reviewSha256,
    runtimeInstance: record.runtimeInstance,
    outcome: "review-revoked",
    managedSequence: 2,
    observedAt: new Date(instant - 1).toISOString(),
  };
  assert.equal(lanReconcilePending(record, "next-owner", proof, instant), null);
  for (const change of [
    { reviewId: "f".repeat(32) },
    { reviewSha256: "f".repeat(64) },
    { runtimeInstance: "f".repeat(32) },
    { runtimeInstance: undefined },
    { managedSequence: 1 },
    { managedSequence: "2" },
    { managedSequence: undefined },
    { operationId: "e".repeat(32) },
    { leaseSha256: "f".repeat(64) },
    { nativeSequence: 5 },
    { observedAt: new Date(instant - 5000).toISOString() },
    { observedAt: new Date(instant + 1).toISOString() },
    { observedAt: "unknown" },
    { outcome: "revoked" },
    { outcome: "unknown" },
  ])
    assert.deepEqual(
      lanReconcilePending(
        record,
        "next-owner",
        { ...proof, ...change },
        instant,
      ),
      record,
    );
  for (const known of [
    { ...record, operationId: "e".repeat(32) },
    { ...record, leaseSha256: "f".repeat(64) },
  ])
    assert.deepEqual(
      lanReconcilePending(known, "next-owner", proof, instant),
      known,
    );
});
test("removal settlement cannot dispatch another remove request", () => {
  assert.equal(lanReviewOperation("activate"), "lan-activate");
  assert.equal(lanReviewOperation("remove"), "lan-remove");
  assert.equal(lanReviewOperation("settle-remove"), "lan-settle-remove");
  assert.equal(lanReviewOperation("cleanup-remove"), "lan-cleanup");
  assert.equal(lanReviewOperation("cleanup-settle"), "lan-cleanup");
  assert.throws(() => lanReviewOperation("settle"));
});

test("only exact fresh native retirement can clear a prior-runtime denial fence", () => {
  const record = {
    ...pending,
    operationId: "e".repeat(32),
    leaseSha256: "f".repeat(64),
  };
  const retired = {
    reviewId: record.reviewId,
    reviewSha256: record.reviewSha256,
    operationId: record.operationId,
    runtimeInstance: record.runtimeInstance,
    leaseSha256: record.leaseSha256,
    outcome: "retired",
    nativeSequence: 5,
    observedAt: new Date(instant - 1).toISOString(),
  };
  assert.equal(
    lanReconcilePending(record, "current-owner", retired, instant),
    null,
  );
  for (const change of [
    { reviewId: "1".repeat(32) },
    { reviewSha256: "1".repeat(64) },
    { operationId: "1".repeat(32) },
    { operationId: undefined },
    { runtimeInstance: "different-owner" },
    { runtimeInstance: undefined },
    { leaseSha256: "1".repeat(64) },
    { leaseSha256: undefined },
    { nativeSequence: 4 },
    { nativeSequence: 6 },
    { nativeSequence: "5" },
    { observedAt: new Date(instant - 5000).toISOString() },
    { observedAt: new Date(instant + 1).toISOString() },
    { observedAt: "unknown" },
    { outcome: "operation" },
    { outcome: "revoked" },
    { outcome: "unknown" },
  ])
    assert.deepEqual(
      lanReconcilePending(
        record,
        "current-owner",
        { ...retired, ...change },
        instant,
      ),
      record,
    );
  assert.deepEqual(lanReconcilePending(record, "", retired, instant), record);
});
const pending = {
  runtimeInstance: "owner-a",
  reviewId: "c".repeat(32),
  reviewSha256: "d".repeat(64),
  purpose: "activate",
};
const operation = {
  reviewId: pending.reviewId,
  operationId: "a".repeat(32),
  leaseSha256: "b".repeat(64),
  sequence: 2,
  phase: "awaiting-admin",
  ingress: "closed",
  firewall: "not-dispatched",
  updatedAt: new Date(instant - 1000).toISOString(),
};
const observe = (record, changes = {}, runtime = "owner-a", connected = true) =>
  lanObservePending(
    record,
    runtime,
    [{ ...operation, ...changes }],
    operation.updatedAt,
    connected,
    instant,
  );

test("lost activation is fenced across absent snapshots, reloads and owner restarts", () => {
  const restored = lanParsePending(JSON.stringify([pending]))[0];
  assert.deepEqual(
    lanObservePending(
      restored,
      "owner-a",
      [],
      operation.updatedAt,
      true,
      instant,
    ),
    pending,
  );
  assert.deepEqual(observe(restored, {}, "owner-b"), pending);
  assert.deepEqual(observe(restored, {}, "owner-a", false), pending);
  assert.deepEqual(observe(restored, { reviewId: "f".repeat(32) }), pending);
  const mapped = observe(restored);
  assert.equal(mapped.operationId, operation.operationId);
  assert.notEqual(mapped, null, "acceptance alone does not settle activation");
  assert.notEqual(
    observe(mapped, {
      phase: "ready",
      ingress: "ready",
      firewall: "create-unknown",
    }),
    null,
  );
  assert.notEqual(
    observe(mapped, {
      phase: "ready",
      ingress: "ready",
      firewall: "created",
      updatedAt: new Date(instant - 5000).toISOString(),
    }),
    null,
  );
  assert.equal(
    observe(mapped, { phase: "ready", ingress: "ready", firewall: "created" }),
    null,
  );
});

test("reconciliation requires the exact review, digest and retained runtime", () => {
  const revoked = {
    reviewId: pending.reviewId,
    reviewSha256: pending.reviewSha256,
    outcome: "revoked",
  };
  assert.equal(lanReconcilePending(pending, "owner-a", revoked), null);
  for (const [runtime, changes] of [
    ["owner-b", {}],
    ["owner-a", { reviewId: "e".repeat(32) }],
    ["owner-a", { reviewSha256: "e".repeat(64) }],
    ["owner-a", { outcome: "unknown" }],
    ["owner-a", { outcome: "operation", operationId: "bad" }],
  ])
    assert.deepEqual(
      lanReconcilePending(pending, runtime, { ...revoked, ...changes }),
      pending,
    );
  const mapped = lanReconcilePending(pending, "owner-a", {
    ...revoked,
    outcome: "operation",
    operationId: operation.operationId,
  });
  assert.equal(mapped.operationId, operation.operationId);
  assert.deepEqual(
    lanReconcilePending(mapped, "owner-a", revoked),
    mapped,
    "a revoked reply cannot erase an already consumed review",
  );
  assert.deepEqual(
    lanReconcilePending(mapped, "owner-a", {
      ...revoked,
      outcome: "operation",
      operationId: "e".repeat(32),
    }),
    mapped,
  );
  assert.notEqual(observe(mapped), null);
});

test("closed recovery settles only positively mapped, known resource outcomes", () => {
  for (const purpose of [
    "activate",
    "remove",
    "settle-remove",
    "cleanup-remove",
    "cleanup-settle",
  ]) {
    const record = { ...pending, purpose };
    for (const firewall of ["create-unknown", "remove-unknown"])
      assert.notEqual(
        observe(record, { phase: "failed", ingress: "closed", firewall }),
        null,
      );
    assert.notEqual(
      observe(record, { phase: "closed", ingress: "unknown" }),
      null,
    );
    assert.notEqual(
      observe(record, { phase: "cleanup-pending", firewall: "created" }),
      null,
    );
    assert.notEqual(observe(record, { phase: "closed", sequence: 0 }), null);
    assert.equal(
      observe(record, { phase: "removed", firewall: "removed" }),
      null,
    );
    assert.equal(
      observe(record, { phase: "closed", firewall: "created" }),
      null,
    );
  }
  assert.notEqual(
    observe(
      { ...pending, purpose: "remove" },
      { phase: "ready", ingress: "ready", firewall: "created" },
    ),
    null,
  );
  // Removal has its own immutable review/operation pair. A contradictory
  // original operation row cannot settle the retained activation fence.
  const mapped = { ...pending, operationId: operation.operationId };
  assert.deepEqual(
    observe(mapped, {
      reviewId: "f".repeat(32),
      phase: "removed",
      firewall: "removed",
    }),
    mapped,
  );
});

test("denial-only recovery storage is strict and bounded without evicting unknowns", () => {
  assert.deepEqual(lanParsePending(null), []);
  assert.deepEqual(lanParsePending(JSON.stringify([pending])), [pending]);
  for (const raw of [
    "{",
    "{}",
    JSON.stringify([pending, pending]),
    " ".repeat(65537),
    JSON.stringify([{ ...pending, nativeAuthority: true }]),
    JSON.stringify([{ ...pending, runtimeInstance: "" }]),
    JSON.stringify([{ ...pending, operationId: "foreign" }]),
    JSON.stringify(
      Array.from({ length: 17 }, (_, i) => ({
        ...pending,
        runtimeInstance: `owner-${i}`,
      })),
    ),
  ])
    assert.throws(() => lanParsePending(raw));
});

test("review refresh retains only exact port lifetime, interface identity, profile and address", () => {
  const publication = {
    containerId: "container-a",
    startedAt: "start-a",
    hostPort: 8080,
    containerPort: 80,
    protocol: "tcp",
    available: true,
  };
  const iface = {
    interfaceGuid: "guid-a",
    interfaceLuid: "7",
    profile: "private",
    available: true,
    privateAddresses: ["192.168.1.2"],
  };
  const options = {
    version: 1,
    complete: true,
    sourceAvailable: true,
    helperAvailable: true,
    readAt: new Date(instant - 1000).toISOString(),
    expiresAt: new Date(instant + 4000).toISOString(),
    publications: [publication],
    interfaces: [iface],
  };
  const choice = {
    publication: lanPublicationKey(publication),
    network: "guid-a/7",
    address: "192.168.1.2",
  };
  assert.equal(lanOptionsCurrent(options, instant), true);
  assert.equal(lanOptionsCurrent(options, instant + 4000), false);
  assert.equal(
    lanOptionsCurrent({ ...options, complete: false }, instant),
    false,
  );
  assert.equal(
    lanOptionsCurrent(
      { ...options, readAt: new Date(instant + 1).toISOString() },
      instant,
    ),
    false,
  );
  assert.equal(
    lanOptionsCurrent(
      { ...options, expiresAt: new Date(instant + 5000).toISOString() },
      instant,
    ),
    false,
  );
  assert.deepEqual(
    lanRetainChoices(
      options,
      {
        ...options,
        publications: [{ ...publication, containerName: "renamed" }],
      },
      choice,
    ),
    choice,
  );
  for (const change of [
    { startedAt: "start-b" },
    { protocol: "udp" },
    { hostPort: 8081 },
    { available: false },
  ])
    assert.equal(
      lanRetainChoices(
        options,
        { ...options, publications: [{ ...publication, ...change }] },
        choice,
      ).publication,
      "",
    );
  for (const change of [
    { interfaceLuid: "8" },
    { profile: "domain" },
    { available: false },
  ]) {
    const next = lanRetainChoices(
      options,
      { ...options, interfaces: [{ ...iface, ...change }] },
      choice,
    );
    assert.equal(next.network, "");
    assert.equal(next.address, "");
  }
  assert.equal(
    lanRetainChoices(
      options,
      {
        ...options,
        interfaces: [{ ...iface, privateAddresses: ["192.168.1.3"] }],
      },
      choice,
    ).address,
    "",
  );
});
