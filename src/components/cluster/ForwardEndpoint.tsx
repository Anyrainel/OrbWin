import { useState } from "react";
import { LocalPortLink } from "../LocalPortLink";
import { forwardEndpoint, type ForwardProtocol } from "./forward-endpoint";

/** A TCP forward does not identify the application protocol or establish trust. */
export function ForwardEndpoint({
  local,
  remote,
}: {
  local: number;
  remote: number;
}) {
  const [protocol, setProtocol] = useState<ForwardProtocol>("tcp");
  const [copyStatus, setCopyStatus] = useState("");
  const endpoint = forwardEndpoint(local, remote, protocol);
  if (!endpoint) return null;
  return (
    <span>
      <code>
        {endpoint.address} → {remote}
      </code>{" "}
      <button
        className="button"
        type="button"
        onClick={() => {
          setCopyStatus("");
          void navigator.clipboard
            .writeText(endpoint.address)
            .then(() => setCopyStatus("Address copied"))
            .catch(() =>
              setCopyStatus(
                `Copy failed. Select the address ${endpoint.address} to copy it.`,
              ),
            );
        }}
        aria-label={`Copy localhost address for remote port ${remote}`}
      >
        Copy address
      </button>{" "}
      <label>
        Open as{" "}
        <select
          value={protocol}
          aria-label={`Protocol for localhost port ${local}`}
          onChange={(event) => {
            const value = event.target.value;
            if (value === "tcp" || value === "http" || value === "https")
              setProtocol(value);
          }}
        >
          <option value="tcp">TCP address only</option>
          <option value="http">HTTP</option>
          <option value="https">HTTPS</option>
        </select>
      </label>{" "}
      {protocol !== "tcp" && (
        <LocalPortLink key={protocol} port={local} scheme={protocol}>
          Open {protocol.toUpperCase()}
        </LocalPortLink>
      )}
      {protocol === "https" && (
        <small>
          {" "}
          TLS passes through unchanged. Your service certificate must cover the
          address used and be trusted by your client. No certificate or trust
          setting is changed.
        </small>
      )}
      {copyStatus && <span role="status"> {copyStatus}</span>}
    </span>
  );
}
