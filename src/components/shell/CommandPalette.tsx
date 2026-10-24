import { useEffect, useMemo, useRef, useState } from "react";
import { Search } from "lucide-react";

export type PaletteCommand = {
  id: string;
  label: string;
  section: string;
  shortcut?: string;
  // A disabled command remains listed with its reason instead of disappearing.
  disabledReason?: string;
  run: () => void;
};

export function matchCommands(commands: PaletteCommand[], query: string) {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
  return commands.filter((command) => {
    const haystack = `${command.section} ${command.label}`.toLowerCase();
    return terms.every((term) => haystack.includes(term));
  });
}

export function CommandPalette({
  commands,
  onClose,
}: {
  commands: PaletteCommand[];
  onClose: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const results = useMemo(
    () => matchCommands(commands, query),
    [commands, query],
  );
  useEffect(() => {
    const trigger = document.activeElement as HTMLElement | null;
    if (dialog.current && !dialog.current.open) dialog.current.showModal();
    input.current?.focus();
    return () => trigger?.focus?.();
  }, []);
  useEffect(() => setActive(0), [query]);
  useEffect(() => {
    list.current
      ?.querySelector(`[data-index="${active}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [active]);
  function choose(command?: PaletteCommand) {
    if (!command || command.disabledReason) return;
    onClose();
    // Run after the dialog has closed so focus moves to the command's target.
    window.setTimeout(command.run, 0);
  }
  const activeId = results[active]
    ? `palette-${results[active].id}`
    : undefined;
  return (
    <dialog
      ref={dialog}
      className="command-palette"
      aria-label="Command palette"
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClick={(event) => {
        if (event.target === dialog.current) onClose();
      }}
    >
      <label className="palette-input">
        <Search size={16} aria-hidden="true" />
        <input
          ref={input}
          role="combobox"
          aria-expanded="true"
          aria-controls="palette-results"
          aria-activedescendant={activeId}
          aria-autocomplete="list"
          aria-label="Search pages and actions"
          placeholder="Search pages and actions"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "ArrowDown") {
              event.preventDefault();
              setActive((index) =>
                results.length ? (index + 1) % results.length : 0,
              );
            } else if (event.key === "ArrowUp") {
              event.preventDefault();
              setActive((index) =>
                results.length
                  ? (index - 1 + results.length) % results.length
                  : 0,
              );
            } else if (event.key === "Home" && !query) {
              event.preventDefault();
              setActive(0);
            } else if (event.key === "End" && !query) {
              event.preventDefault();
              setActive(Math.max(0, results.length - 1));
            } else if (event.key === "Enter") {
              event.preventDefault();
              choose(results[active]);
            }
          }}
        />
        <kbd>Esc</kbd>
      </label>
      <ul
        ref={list}
        id="palette-results"
        role="listbox"
        aria-label="Matching pages and actions"
      >
        {results.map((command, index) => (
          <li
            key={command.id}
            id={`palette-${command.id}`}
            role="option"
            data-index={index}
            aria-selected={index === active}
            aria-disabled={command.disabledReason ? true : undefined}
            className={`palette-option ${index === active ? "active" : ""}`}
            onMouseMove={() => setActive(index)}
            onClick={() => choose(command)}
          >
            <span className="palette-section">{command.section}</span>
            <span className="palette-label">
              {command.label}
              {command.disabledReason && (
                <small>{command.disabledReason}</small>
              )}
            </span>
            {command.shortcut && <kbd>{command.shortcut}</kbd>}
          </li>
        ))}
      </ul>
      {results.length === 0 && (
        <p className="palette-empty" role="status">
          No pages or actions match “{query}”.
        </p>
      )}
    </dialog>
  );
}
