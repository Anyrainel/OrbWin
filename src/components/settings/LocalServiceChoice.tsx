import { useEffect, useId, useState } from "react";
import type {
  LocalServicePreference,
  LocalServiceSelector,
} from "../../backend";

export function sameServiceSelector(
  a: LocalServiceSelector,
  b: LocalServiceSelector,
) {
  return (
    a.containerId === b.containerId &&
    a.project === b.project &&
    a.service === b.service &&
    a.replica === b.replica
  );
}

export function LocalServiceChoice({
  selector,
  preference,
  busy,
  save,
}: {
  selector: LocalServiceSelector;
  preference?: LocalServicePreference;
  busy: boolean;
  save: (preference: LocalServicePreference | null) => Promise<void>;
}) {
  const id = useId();
  const [name, setName] = useState(preference?.name ?? "");
  const [port, setPort] = useState(
    preference?.port ? String(preference.port) : "",
  );
  const [disabled, setDisabled] = useState(preference?.disabled ?? false);
  const routesKey = JSON.stringify(preference?.routes ?? []);
  const [routes, setRoutes] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    setName(preference?.name ?? "");
    setPort(preference?.port ? String(preference.port) : "");
    setDisabled(preference?.disabled ?? false);
    setRoutes(
      (preference?.routes ?? [])
        .map(
          (route) =>
            `${route.name}@${route.port}${route.https ? ":https" : ""}`,
        )
        .join("\n"),
    );
    setError("");
  }, [preference?.name, preference?.port, preference?.disabled, routesKey]);
  async function apply() {
    if (routes.trim()) {
      const entries = routes
        .trim()
        .split(/\r?\n/)
        .map((line) => line.trim());
      const parsed: { name: string; port: number; https?: boolean }[] = [];
      const seen = new Set<string>();
      for (const entry of entries) {
        const match = /^(.*)@([1-9][0-9]{0,4})(:https)?$/.exec(entry);
        if (
          !match ||
          Number(match[2]) > 65535 ||
          match[1].length > 253 ||
          !/^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+orbwin\.(?:localhost|test)$/.test(
            match[1],
          ) ||
          seen.has(match[1]) ||
          entries.length > 16
        ) {
          setError(
            "Enter up to sixteen distinct exact names, one per line: app.orbwin.localhost@8080 or api.orbwin.test@8443:https.",
          );
          return;
        }
        seen.add(match[1]);
        parsed.push({
          name: match[1],
          port: Number(match[2]),
          ...(match[3] ? { https: true } : {}),
        });
      }
      setError("");
      await save({ selector, disabled, routes: parsed });
      return;
    }
    const number = port === "" ? 0 : Number(port);
    if (
      !Number.isInteger(number) ||
      number < 0 ||
      number > 65535 ||
      (port !== "" && number === 0)
    ) {
      setError(
        "Choose a container HTTP port from 1 to 65535, or leave it blank for discovery.",
      );
      return;
    }
    const selected = name.trim();
    if (
      selected &&
      (selected.length > 253 ||
        !/^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+orbwin\.(?:localhost|test)$/.test(
          selected,
        ))
    ) {
      setError(
        "Use a lowercase DNS name ending in .orbwin.localhost or .orbwin.test.",
      );
      return;
    }
    setError("");
    await save({
      selector,
      disabled,
      ...(selected ? { name: selected } : {}),
      ...(number ? { port: number } : {}),
    });
  }
  return (
    <details>
      <summary>Routing choices</summary>
      <p>
        {selector.containerId
          ? "Applies to this exact container. A replacement container gets its own choices."
          : `Applies to Compose ${selector.project} / ${selector.service}, replica ${selector.replica}, including recreation.`}
      </p>
      <div className="action-row">
        <label htmlFor={`${id}-name`}>Local name</label>
        <input
          id={`${id}-name`}
          value={name}
          disabled={busy || !!routes.trim()}
          placeholder="Automatic"
          onChange={(event) => setName(event.target.value)}
        />
        <label htmlFor={`${id}-port`}>Container HTTP port</label>
        <input
          id={`${id}-port`}
          type="number"
          min="1"
          max="65535"
          value={port}
          disabled={busy || !!routes.trim()}
          placeholder="Automatic"
          onChange={(event) => setPort(event.target.value)}
        />
        <label>
          <input
            type="checkbox"
            checked={disabled}
            disabled={busy}
            onChange={(event) => setDisabled(event.target.checked)}
          />{" "}
          Disable this route
        </label>
        <button className="button" disabled={busy} onClick={() => void apply()}>
          Save routing choices
        </button>
        {preference && (
          <button
            className="button"
            disabled={busy}
            onClick={() => void save(null)}
          >
            Use discovered defaults
          </button>
        )}
      </div>
      <label htmlFor={`${id}-routes`}>Exact names and container ports</label>
      <textarea
        id={`${id}-routes`}
        rows={4}
        value={routes}
        disabled={busy}
        placeholder={"app.orbwin.localhost@8080\napi.orbwin.test@8443:https"}
        onChange={(event) => setRoutes(event.target.value)}
      />
      <p>
        Each line selects one exact name and internal port. Include both
        .orbwin.localhost and .orbwin.test names when both client groups need
        access. These choices replace routing labels. Clear all fields to use
        labels and discovery. HTTPS backends require a certificate trusted by
        Windows and valid for the exact name; verification cannot be bypassed.
        Public HTTP links redirect to HTTPS.
      </p>
      {error && <p role="alert">{error}</p>}
    </details>
  );
}
