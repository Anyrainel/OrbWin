import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { CheckCircle2, Info, TriangleAlert, X } from "lucide-react";

export type ToastTone = "success" | "info" | "error";
type Toast = { id: number; tone: ToastTone; message: string };
type Notify = (tone: ToastTone, message: string) => void;
const Context = createContext<Notify>(() => {});
export function useToast() {
  return useContext(Context);
}

// Success/info messages dismiss after a delay; errors persist until dismissed
// so a failure is never lost to a timer. Both live regions always exist in the
// DOM so assistive technology announces newly inserted messages.
const lifetime = 6000;
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const next = useRef(0);
  const timers = useRef(new Map<number, number>());
  const dismiss = useCallback((id: number) => {
    window.clearTimeout(timers.current.get(id));
    timers.current.delete(id);
    setToasts((items) => items.filter((item) => item.id !== id));
  }, []);
  const notify = useCallback<Notify>(
    (tone, message) => {
      const id = ++next.current;
      setToasts((items) =>
        [
          ...items.filter(
            (item) => item.message !== message || item.tone !== tone,
          ),
          { id, tone, message },
        ].slice(-4),
      );
      if (tone !== "error")
        timers.current.set(
          id,
          window.setTimeout(() => dismiss(id), lifetime),
        );
    },
    [dismiss],
  );
  useEffect(() => {
    const pending = timers.current;
    return () => pending.forEach((timer) => window.clearTimeout(timer));
  }, []);
  const render = (items: Toast[]) =>
    items.map((toast) => (
      <div key={toast.id} className={`toast ${toast.tone}`}>
        {toast.tone === "error" ? (
          <TriangleAlert size={15} aria-hidden="true" />
        ) : toast.tone === "success" ? (
          <CheckCircle2 size={15} aria-hidden="true" />
        ) : (
          <Info size={15} aria-hidden="true" />
        )}
        <span>{toast.message}</span>
        <button
          aria-label="Dismiss notification"
          onClick={() => dismiss(toast.id)}
        >
          <X size={13} />
        </button>
      </div>
    ));
  return (
    <Context.Provider value={notify}>
      {children}
      <div className="toast-stack">
        <div role="alert" aria-live="assertive">
          {render(toasts.filter((toast) => toast.tone === "error"))}
        </div>
        <div role="status" aria-live="polite">
          {render(toasts.filter((toast) => toast.tone !== "error"))}
        </div>
      </div>
    </Context.Provider>
  );
}
