import { useEffect, useState } from "react";
import { ToastProvider } from "./ui";
import { Roster } from "./Roster";
import { SubjectPanel } from "./SubjectPanel";

type Theme = "dark" | "light";

function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem("fulcrum-theme") as Theme | null;
    if (saved) return saved;
    return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  });
  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("fulcrum-theme", theme);
  }, [theme]);
  return [theme, () => setTheme((t) => (t === "dark" ? "light" : "dark"))];
}

export function App() {
  const [theme, toggleTheme] = useTheme();
  const [selected, setSelected] = useState<number | null>(null);

  return (
    <ToastProvider>
      <div className="min-h-screen">
        <header className="scanlines border-b border-edge">
          <div className="mx-auto max-w-5xl px-6 py-5 flex items-center justify-between">
            <div className="flex items-baseline gap-3">
              <span className="stamp text-signal text-xl leading-none">FULCRUM</span>
              <span className="data text-[11px] text-haze hidden sm:inline">
                persons-of-interest registry
              </span>
            </div>
            <button
              onClick={toggleTheme}
              className="stamp text-[11px] text-haze hover:text-signal transition-colors"
              aria-label="Toggle theme"
            >
              {theme === "dark" ? "◐ light" : "◑ dark"}
            </button>
          </div>
        </header>

        <main className="mx-auto max-w-5xl px-6 py-10">
          {selected === null ? (
            <Roster onOpen={setSelected} />
          ) : (
            <SubjectPanel id={selected} onBack={() => setSelected(null)} />
          )}
        </main>

        <footer className="mx-auto max-w-5xl px-6 py-8 text-haze data text-[11px] border-t border-edge">
          Only enrolled subjects are ever stored · non-matching media is discarded
        </footer>
      </div>
    </ToastProvider>
  );
}
