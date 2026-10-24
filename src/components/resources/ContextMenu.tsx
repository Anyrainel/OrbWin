import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { Action } from "./actions";
import { LocalPortLink } from "../LocalPortLink";

export type MenuRequest = {
  x: number;
  y: number;
  label: string;
  actions: Action[];
  returnFocus: HTMLElement | null;
};

// Menu-button pattern: focus moves into the menu, arrows/Home/End move between
// items, Escape/Tab close it and restore focus to the invoking row.
export function ContextMenu({
  menu,
  onRun,
  onClose,
}: {
  menu: MenuRequest;
  onRun: (action: Action) => void;
  onClose: (restoreFocus: boolean) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({ left: menu.x, top: menu.y });
  const items = () =>
    [
      ...(ref.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ??
        []),
    ] as HTMLElement[];
  useLayoutEffect(() => {
    const box = ref.current?.getBoundingClientRect();
    if (box)
      setPosition({
        left: Math.max(4, Math.min(menu.x, window.innerWidth - box.width - 4)),
        top: Math.max(4, Math.min(menu.y, window.innerHeight - box.height - 4)),
      });
    items()[0]?.focus();
  }, [menu]);
  useEffect(() => {
    const outside = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) onClose(false);
    };
    const dismiss = () => onClose(false);
    window.addEventListener("pointerdown", outside, true);
    window.addEventListener("resize", dismiss);
    window.addEventListener("blur", dismiss);
    return () => {
      window.removeEventListener("pointerdown", outside, true);
      window.removeEventListener("resize", dismiss);
      window.removeEventListener("blur", dismiss);
    };
  }, [onClose]);
  return (
    <div
      ref={ref}
      role="menu"
      aria-label={menu.label}
      className="context-menu"
      style={position}
      onKeyDown={(event) => {
        const list = items();
        const index = list.indexOf(document.activeElement as HTMLElement);
        let next = -1;
        if (event.key === "ArrowDown") next = (index + 1) % list.length;
        else if (event.key === "ArrowUp")
          next = (index - 1 + list.length) % list.length;
        else if (event.key === "Home") next = 0;
        else if (event.key === "End") next = list.length - 1;
        else if (event.key === "Escape" || event.key === "Tab") {
          event.preventDefault();
          event.stopPropagation();
          onClose(true);
          return;
        } else return;
        event.preventDefault();
        list[next]?.focus();
      }}
    >
      {menu.actions.map((action) =>
        action.port ? (
          <LocalPortLink
            key={action.id}
            role="menuitem"
            tabIndex={-1}
            port={action.port}
            onOpened={() => onClose(true)}
          >
            {action.label}
          </LocalPortLink>
        ) : (
          <button
            key={action.id}
            role="menuitem"
            tabIndex={-1}
            className={action.destructive ? "danger" : ""}
            aria-disabled={action.unavailable ? true : undefined}
            title={action.unavailable}
            onClick={() => {
              if (action.unavailable) return;
              onClose(true);
              onRun(action);
            }}
          >
            {action.label}
            {action.unavailable && <small>{action.unavailable}</small>}
          </button>
        ),
      )}
    </div>
  );
}
