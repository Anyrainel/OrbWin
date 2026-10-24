import { useState, type AnchorHTMLAttributes } from "react";
import { invoke, isTauri } from "@tauri-apps/api/core";

export function LocalPortLink({
  port,
  onOpened,
  children,
  ...props
}: Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "href" | "onClick"> & {
  port: number;
  onOpened?: () => void;
}) {
  const [error, setError] = useState("");
  return (
    <>
      <a
        {...props}
        href={`http://127.0.0.1:${port}/`}
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
          void invoke("open_local_port", { port })
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
