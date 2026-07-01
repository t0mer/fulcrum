import { useEffect, useState } from "react";
import { api, auth, getAuthInfo } from "./api";
import { Button, Spinner, TextInput, ToastProvider } from "./ui";
import { Roster } from "./Roster";
import { SubjectPanel } from "./SubjectPanel";
import { Groups } from "./Groups";
import { Matches } from "./Matches";
import { Settings } from "./Settings";

type Theme = "dark" | "light";
type View = "subjects" | "groups" | "matches" | "settings";
type Gate = "checking" | "login" | "open";

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
  { id: "settings", label: "settings" },
];

export function App() {
  const [theme, toggleTheme] = useTheme();
  const [view, setView] = useState<View>("subjects");
  const [selected, setSelected] = useState<number | null>(null);
  const [gate, setGate] = useState<Gate>("checking");
  const [authRequired, setAuthRequired] = useState(false);

  useEffect(() => {
    (async () => {
      const info = await getAuthInfo().catch(() => ({ auth_required: false }));
      setAuthRequired(info.auth_required);
      if (!info.auth_required) return setGate("open");
      if (auth.get()) {
        try {
          await api.getSettings(); // validates the stored token
          return setGate("open");
        } catch {
          auth.clear();
        }
      }
      setGate("login");
    })();
  }, []);

  const go = (v: View) => {
    setSelected(null);
    setView(v);
  };

  if (gate === "checking") {
    return (
      <div className="min-h-screen grid place-items-center">
        <Spinner label="Connecting…" />
      </div>
    );
  }
  if (gate === "login") {
    return <Login onOk={() => setGate("open")} />;
  }

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
              {authRequired && (
                <button
                  onClick={() => {
                    auth.clear();
                    window.location.reload();
                  }}
                  className="stamp text-[11px] text-haze hover:text-signal transition-colors"
                >
                  sign out
                </button>
              )}
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
          ) : view === "matches" ? (
            <Matches onOpenSubject={setSelected} />
          ) : (
            <Settings />
          )}
        </main>

        <footer className="mx-auto max-w-5xl px-6 py-8 text-haze data text-[11px] border-t border-edge">
          Only enrolled subjects are ever stored · non-matching media is discarded
        </footer>
      </div>
    </ToastProvider>
  );
}

function Login({ onOk }: { onOk: () => void }) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const submit = async () => {
    setBusy(true);
    setErr("");
    auth.set(token.trim());
    try {
      await api.getSettings();
      onOk();
    } catch {
      auth.clear();
      setErr("That token was not accepted.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="min-h-screen grid place-items-center p-4">
      <div className="dossier w-full max-w-sm p-8">
        <div className="stamp text-signal text-xl mb-1">FULCRUM</div>
        <p className="data text-[11px] text-haze mb-6">restricted · access token required</p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (token.trim()) submit();
          }}
        >
          <TextInput
            label="API token"
            type="password"
            value={token}
            autoFocus
            onChange={(e) => setToken(e.target.value)}
          />
          {err && <p className="text-red-400 text-xs mt-2">{err}</p>}
          <Button
            variant="primary"
            className="w-full mt-5"
            type="submit"
            disabled={busy || !token.trim()}
          >
            {busy ? "Verifying…" : "Unlock"}
          </Button>
        </form>
      </div>
    </div>
  );
}
