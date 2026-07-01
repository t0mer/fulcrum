import { useEffect, useState } from "react";
import { ToastProvider } from "./ui";
import { Roster } from "./Roster";
import { SubjectPanel } from "./SubjectPanel";
import { Groups } from "./Groups";
import { Matches } from "./Matches";

type Theme = "dark" | "light";
type View = "subjects" | "groups" | "matches";

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

const TABS: { id: View; label: string }[] = [
  { id: "subjects", label: "registry" },
  { id: "groups", label: "channels" },
  { id: "matches", label: "watch" },
];

export function App() {
  const [theme, toggleTheme] = useTheme();
  const [view, setView] = useState<View>("subjects");
  const [selected, setSelected] = useState<number | null>(null);

  const go = (v: View) => {
    setSelected(null);
    setView(v);
  };

  return (
    <ToastProvider>
      <div className="min-h-screen">
        <header className="scanlines border-b border-edge">
          <div className="mx-auto max-w-5xl px-6 py-5 flex items-center justify-between gap-4">
            <div className="flex items-baseline gap-3">
              <span className="stamp text-signal text-xl leading-none">FULCRUM</span>
              <span className="data text-[11px] text-haze hidden sm:inline">
                persons-of-interest registry
              </span>
            </div>
            <div className="flex items-center gap-5">
              <nav className="flex gap-4">
                {TABS.map((t) => (
                  <button
                    key={t.id}
                    onClick={() => go(t.id)}
                    className={
                      "stamp text-[11px] transition-colors " +
                      (view === t.id && selected === null
                        ? "text-signal"
                        : "text-haze hover:text-bone")
                    }
                  >
                    {t.label}
                  </button>
                ))}
              </nav>
              <button
                onClick={toggleTheme}
                className="stamp text-[11px] text-haze hover:text-signal transition-colors"
                aria-label="Toggle theme"
              >
                {theme === "dark" ? "◐" : "◑"}
              </button>
            </div>
          </div>
        </header>

        <main className="mx-auto max-w-5xl px-6 py-10">
          {selected !== null ? (
            <SubjectPanel id={selected} onBack={() => setSelected(null)} />
          ) : view === "subjects" ? (
            <Roster onOpen={setSelected} />
          ) : view === "groups" ? (
            <Groups />
          ) : (
            <Matches onOpenSubject={setSelected} />
          )}
        </main>

        <footer className="mx-auto max-w-5xl px-6 py-8 text-haze data text-[11px] border-t border-edge">
          Only enrolled subjects are ever stored · non-matching media is discarded
        </footer>
      </div>
    </ToastProvider>
  );
}
