import { useState, type AnchorHTMLAttributes } from "react";
import { invoke, isTauri } from "@tauri-apps/api/core";

export function LocalPortLink({
  port,
  scheme = "http",
  onOpened,
  children,
  ...props
}: Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href" | "onClick"> & {
  port: number;
  scheme?: "http" | "https";
  onOpened?: () => void;
}) {
  const [error, setError] = useState("");
  if (
    !Number.isInteger(port) ||
    port < 1 ||
    port > 65535 ||
    (scheme !== "http" && scheme !== "https")
  )
    return null;
  return (
    <>
      <a
        {...props}
        href={`${scheme}://127.0.0.1:${port}/`}
        target="_blank"
        rel="noreferrer noopener"
        onClick={(event) => {
          event.stopPropagation();
          if (!isTauri()) {
            onOpened?.();
            return;
          }
          event.preventDefault();
          setError("");
          void invoke("open_local_port", { port, scheme })
            .then(() => onOpened?.())
            .catch((cause) => setError(String(cause)));
        }}
      >
        {children}
      </a>
      {error && <span role="alert">{error}</span>}
    </>
  );
}
