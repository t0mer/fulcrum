import { createContext, useCallback, useContext, useEffect, useState } from "react";
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";

type Variant = "primary" | "ghost" | "danger";

export function Button({
  variant = "ghost",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  const base =
    "inline-flex items-center justify-center gap-2 px-4 h-10 text-sm font-medium " +
    "transition-colors disabled:opacity-40 disabled:cursor-not-allowed select-none";
  const styles: Record<Variant, string> = {
    primary: "bg-signal text-white hover:brightness-110",
    ghost: "border border-edge text-bone hover:border-signal hover:text-signal bg-transparent",
    danger: "border border-edge text-haze hover:border-red-500 hover:text-red-400 bg-transparent",
  };
  return <button className={`${base} ${styles[variant]} ${className}`} {...props} />;
}

export function TextInput({
  label,
  hint,
  className = "",
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string }) {
  return (
    <label className="block">
      <span className="stamp block text-[11px] text-haze mb-1.5">{label}</span>
      <input
        className={
          "w-full h-10 px-3 bg-panel-2 border border-edge text-bone placeholder:text-haze/50 " +
          "focus:border-signal outline-none data " +
          className
        }
        {...props}
      />
      {hint && <span className="block text-xs text-haze mt-1">{hint}</span>}
    </label>
  );
}

export function Modal({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/60 backdrop-blur-sm p-4"
      onClick={onClose}
    >
      <div
        className="dossier w-full max-w-md p-6 rise"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        <h2 className="stamp text-signal text-sm mb-5">{title}</h2>
        {children}
      </div>
    </div>
  );
}

export function Spinner({ label }: { label?: string }) {
  return (
    <span className="inline-flex items-center gap-2 text-haze text-sm">
      <span className="h-3 w-3 border-2 border-edge border-t-signal rounded-full animate-spin" />
      {label}
    </span>
  );
}

/* --- Toasts --- */
type Toast = { id: number; msg: string; kind: "ok" | "err" };
const ToastCtx = createContext<(msg: string, kind?: "ok" | "err") => void>(() => {});
export const useToast = () => useContext(ToastCtx);

let toastSeq = 0;
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((msg: string, kind: "ok" | "err" = "ok") => {
    const id = ++toastSeq;
    setToasts((t) => [...t, { id, msg, kind }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4000);
  }, []);

  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed bottom-5 right-5 z-[60] flex flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={
              "dossier px-4 py-3 text-sm max-w-xs rise border-l-2 " +
              (t.kind === "ok" ? "border-l-[color:var(--ready)]" : "border-l-red-500")
            }
          >
            {t.msg}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}
