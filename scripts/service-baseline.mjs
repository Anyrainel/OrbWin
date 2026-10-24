import { createConnection } from "node:net";
import { resolve } from "node:path";

// Runs only after prove-guest verified registration ownership, manifest, and
// daemon identity. Every mutation is scoped to a freshly generated test project.
export async function runBaseline({ root, runId, docker, evidence }) {
  const project = `orbwin-baseline-${runId.slice(0, 8)}`;
  const tag = `orbwin-baseline:${runId.slice(0, 8)}`;
  const compose = (...args) =>
    docker([
      "compose",
      "-p",
      project,
      "-f",
      resolve(root, "tests/workloads/compose.yaml"),
      ...args,
    ]);
  let imageBuilt = false;
  let failure;
  try {
    docker(["compose", "version"]);
    docker(["buildx", "version"]);
    compose("up", "-d", "--wait", "--wait-timeout", "90");
    if (compose("exec", "-T", "redis", "redis-cli", "ping").trim() !== "PONG")
      throw new Error("Redis response differed");
    compose(
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      "postgres",
      "-d",
      "orbwin_test",
      "-v",
      "ON_ERROR_STOP=1",
      "-c",
      "CREATE TABLE proof(value integer); INSERT INTO proof VALUES (42);",
    );
    compose("exec", "-T", "redis", "redis-cli", "set", "proof", project);
    const binding = compose("port", "redis", "6379").trim();
    const match = binding.match(/^127\.0\.0\.1:(\d+)$/);
    if (!match)
      throw new Error(`Unexpected Redis loopback binding: ${binding}`);
    const response = await new Promise((accept, reject) => {
      const connection = createConnection({
        host: "127.0.0.1",
        port: Number(match[1]),
      });
      const timer = setTimeout(() => {
        connection.destroy();
        reject(new Error("Published Redis port timed out"));
      }, 5000);
      connection.on("connect", () => connection.write("*1\r\n$4\r\nPING\r\n"));
      connection.once("data", (bytes) => {
        clearTimeout(timer);
        connection.destroy();
        accept(bytes.toString());
      });
      connection.once("error", (error) => {
        clearTimeout(timer);
        connection.destroy();
        reject(error);
      });
    });
    if (response !== "+PONG\r\n")
      throw new Error("Published port did not route to Redis");
    evidence.publishedPort = { binding, response };
    compose("restart", "redis", "postgres");
    compose("up", "-d", "--wait", "--wait-timeout", "90");
    if (
      compose("exec", "-T", "redis", "redis-cli", "get", "proof").trim() !==
      project
    )
      throw new Error("Redis data did not survive restart");
    if (
      compose(
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "postgres",
        "-d",
        "orbwin_test",
        "-Atc",
        "SELECT value FROM proof;",
      ).trim() !== "42"
    )
      throw new Error("Postgres data did not survive restart");
    compose("logs", "--no-color", "--tail", "30");
    docker([
      "buildx",
      "build",
      "--builder",
      "default",
      "--load",
      "-t",
      tag,
      resolve(root, "tests/workloads/build"),
    ]);
    imageBuilt = true;
    if (docker(["run", "--rm", tag]).trim() !== "orbwin-build-proof")
      throw new Error("Built image output differed");
    evidence.serviceBaselinePassed = true;
    if (process.argv.includes("--observe")) {
      console.log(
        "Service baseline passed. Keeping owned services available for UI inspection for 90 seconds, then cleaning them up.",
      );
      await new Promise((resolve) => setTimeout(resolve, 90000));
    }
  } catch (error) {
    failure = error;
  } finally {
    try {
      compose("down", "--volumes", "--remove-orphans", "--timeout", "15");
    } catch (error) {
      failure ??= error;
    }
    if (imageBuilt) {
      try {
        docker(["image", "rm", tag]);
      } catch (error) {
        failure ??= error;
      }
    }
  }
  if (failure) throw failure;
}
