export async function waitForEngine(
  probe,
  {
    timeoutMs = 60000,
    intervalMs = 500,
    delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
  } = {},
) {
  const deadline = performance.now() + timeoutMs;
  let lastError;
  do {
    try {
      return probe();
    } catch (error) {
      const message = String(error.stderr ?? error.message);
      if (
        !/failed to connect|cannot find the file|connection refused|pipe is being closed/i.test(
          message,
        )
      )
        throw error;
      lastError = error;
    }
    await delay(intervalMs);
  } while (performance.now() < deadline);
  throw new Error(
    `Owned engine endpoint did not become reachable within ${timeoutMs}ms: ${lastError?.message}`,
  );
}
