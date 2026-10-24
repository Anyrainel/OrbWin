// Only for read-only operations rejected before dispatch by the bounded client
// registry. Identity, transport and command failures are never retried here.
const busy =
  "Another operation is active. Wait for it to finish before retrying.";
export async function retryBusyRead<T>(
  read: () => Promise<T>,
  signal: AbortSignal,
  delays = [100, 300, 600],
): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    signal.throwIfAborted();
    try {
      return await read();
    } catch (error) {
      if (
        String(error).replace(/^Error: /, "") !== busy ||
        attempt >= delays.length
      )
        throw error;
      await new Promise<void>((resolve, reject) => {
        const finish = () => {
          signal.removeEventListener("abort", cancel);
          resolve();
        };
        const cancel = () => {
          globalThis.clearTimeout(timer);
          signal.removeEventListener("abort", cancel);
          reject(signal.reason);
        };
        const timer = globalThis.setTimeout(finish, delays[attempt]);
        signal.addEventListener("abort", cancel, { once: true });
        if (signal.aborted) cancel();
      });
    }
  }
}
