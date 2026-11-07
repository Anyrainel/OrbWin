import { samplingIntervals } from "./stats-model.ts";

export type ActivityEvent<T> =
  | { kind: "reading" | "cancelling" | "cancelled" | "settled" }
  | { kind: "sample"; sample: T }
  | { kind: "error"; message: string };
export type ActivityClock = {
  defer: (callback: () => void) => () => void;
  repeat: (callback: () => void, milliseconds: number) => () => void;
  visible: () => boolean;
};

// One mounted view owns the timer and one native read until it has settled.
// Abort invalidates its result immediately but does not release its slot early.
// Disposal suppresses all late events, including clients that ignore abort.
export class ActivitySampler<T> {
  private active: AbortController | null = null;
  private disposed = false;
  private stopTimer: (() => void) | null = null;
  private stopInitial: () => void;
  private readonly fetchSample: (signal: AbortSignal) => Promise<T>;
  private readonly notify: (event: ActivityEvent<T>) => void;
  private readonly clock: ActivityClock;
  constructor(
    fetchSample: (signal: AbortSignal) => Promise<T>,
    notify: (event: ActivityEvent<T>) => void,
    clock: ActivityClock,
  ) {
    this.fetchSample = fetchSample;
    this.notify = notify;
    this.clock = clock;
    this.stopInitial = clock.defer(() => this.tick());
  }
  configure(live: boolean, interval: number) {
    if (!samplingIntervals.some((value) => value === interval))
      throw new Error("Unsupported activity sampling interval.");
    this.stopTimer?.();
    this.stopTimer = null;
    if (live && !this.disposed)
      this.stopTimer = this.clock.repeat(() => this.tick(), interval);
  }
  private tick() {
    if (this.clock.visible()) void this.read();
  }
  async read() {
    if (this.disposed || this.active) return;
    const controller = new AbortController();
    this.active = controller;
    this.notify({ kind: "reading" });
    try {
      const sample = await this.fetchSample(controller.signal);
      if (!this.disposed && !controller.signal.aborted)
        this.notify({ kind: "sample", sample });
    } catch (error) {
      if (!this.disposed && !controller.signal.aborted)
        this.notify({ kind: "error", message: String(error) });
    } finally {
      this.active = null;
      if (!this.disposed)
        this.notify({
          kind: controller.signal.aborted ? "cancelled" : "settled",
        });
    }
  }
  cancel() {
    this.stopTimer?.();
    this.stopTimer = null;
    this.stopInitial();
    if (this.active && !this.active.signal.aborted) {
      if (!this.disposed) this.notify({ kind: "cancelling" });
      this.active.abort();
    }
  }
  dispose() {
    this.disposed = true;
    this.cancel();
  }
}
