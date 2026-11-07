import test from "node:test";
import assert from "node:assert/strict";
import { ActivitySampler } from "../src/components/resources/activity-sampler.ts";

function fixture() {
  const jobs = new Set();
  let visible = true;
  const add = (callback, milliseconds) => {
    const job = { callback, milliseconds };
    jobs.add(job);
    return () => jobs.delete(job);
  };
  const events = [],
    calls = [];
  const owner = new ActivitySampler(
    (signal) =>
      new Promise((resolve, reject) => calls.push({ signal, resolve, reject })),
    (event) => events.push(event),
    {
      defer: (callback) => add(callback, 0),
      repeat: add,
      visible: () => visible,
    },
  );
  const tick = () => {
    for (const job of [...jobs]) {
      if (job.milliseconds === 0) jobs.delete(job);
      job.callback();
    }
  };
  return {
    owner,
    events,
    calls,
    jobs,
    tick,
    hide: () => {
      visible = false;
    },
    show: () => {
      visible = true;
    },
  };
}
const flush = async () => {
  await Promise.resolve();
  await Promise.resolve();
};

test("cancel retains the active slot until acknowledgement and discards even successful late data", async () => {
  const f = fixture();
  f.owner.configure(true, 3000);
  const pending = f.owner.read();
  f.owner.cancel();
  assert.equal(f.calls[0].signal.aborted, true);
  assert.equal(f.jobs.size, 0);
  await f.owner.read();
  assert.equal(f.calls.length, 1);
  assert.equal(f.events.at(-1).kind, "cancelling");
  f.calls[0].resolve({ cpu: 999 });
  await pending;
  assert.equal(f.events.at(-1).kind, "cancelled");
  assert.equal(
    f.events.some((event) => event.kind === "sample"),
    false,
  );
  const next = f.owner.read();
  f.calls[1].resolve({ cpu: 1 });
  await next;
  assert.deepEqual(
    f.events.filter((event) => event.kind === "sample"),
    [{ kind: "sample", sample: { cpu: 1 } }],
  );
  f.owner.dispose();
});
test("hidden ticks and busy ticks do not dispatch; changing intervals replaces the timer", async () => {
  const f = fixture();
  f.owner.configure(true, 5000);
  f.hide();
  f.tick();
  assert.equal(f.calls.length, 0);
  f.show();
  f.tick();
  f.tick();
  assert.equal(f.calls.length, 1);
  f.owner.configure(true, 10000);
  assert.deepEqual(
    [...f.jobs].map((job) => job.milliseconds),
    [10000],
  );
  assert.throws(() => f.owner.configure(true, 1), /Unsupported/);
  assert.deepEqual(
    [...f.jobs].map((job) => job.milliseconds),
    [10000],
  );
  f.calls[0].resolve(1);
  await flush();
  f.owner.configure(false, 10000);
  f.tick();
  assert.equal(f.calls.length, 1);
  const manual = f.owner.read();
  f.calls[1].resolve(2);
  await manual;
  assert.equal(f.calls.length, 2);
  f.owner.dispose();
});
test("view disposal cancels work and prevents all late events and future reads", async () => {
  const f = fixture();
  const pending = f.owner.read();
  const eventCount = f.events.length;
  f.owner.dispose();
  f.calls[0].reject(new Error("late failure"));
  await pending;
  assert.equal(f.calls[0].signal.aborted, true);
  assert.equal(f.events.length, eventCount);
  f.owner.configure(true, 5000);
  f.tick();
  await f.owner.read();
  assert.equal(f.calls.length, 1);
  assert.equal(f.jobs.size, 0);
});
test("failed sampling settles and permits recovery without a fabricated sample or eager retry", async () => {
  const f = fixture();
  f.owner.configure(true, 3000);
  const first = f.owner.read();
  f.calls[0].reject(new Error("stats permission denied"));
  await first;
  assert.equal(f.calls.length, 1);
  assert.deepEqual(
    f.events.map((event) => event.kind),
    ["reading", "error", "settled"],
  );
  assert.match(f.events[1].message, /permission denied/);
  f.tick();
  f.calls[1].resolve(5);
  await flush();
  assert.equal(f.events.filter((event) => event.kind === "sample").length, 1);
  f.owner.dispose();
});
