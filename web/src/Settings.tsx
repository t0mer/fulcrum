import { useEffect, useState } from "react";
import { api, ApiError, type Settings as SettingsData } from "./api";
import { Button, Spinner, TextInput, useToast } from "./ui";

const MODES: { id: SettingsData["sink_mode"]; label: string; hint: string }[] = [
  { id: "both", label: "Both", hint: "Save the match and forward it" },
  { id: "storage-only", label: "Storage only", hint: "Save; don't forward" },
  { id: "forward-only", label: "Forward only", hint: "Forward; don't save" },
];

export function Settings() {
  const [settings, setSettings] = useState<SettingsData | null>(null);
  const [threshold, setThreshold] = useState("");
  const [mode, setMode] = useState<SettingsData["sink_mode"]>("both");
  const [saving, setSaving] = useState(false);
  const toast = useToast();

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        setSettings(s);
        setThreshold(String(s.global_threshold));
        setMode(s.sink_mode);
      })
      .catch((e) => toast(e.message, "err"));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const save = async () => {
    setSaving(true);
    try {
      const s = await api.updateSettings({ global_threshold: Number(threshold), sink_mode: mode });
      setSettings(s);
      toast("Settings saved");
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Save failed", "err");
    } finally {
      setSaving(false);
    }
  };

  if (!settings) return <Spinner label="Loading settings…" />;

  const thresholdValid = Number(threshold) > 0 && Number(threshold) < 1;

  return (
    <section className="max-w-xl">
      <p className="stamp text-[11px] text-haze mb-2">configuration</p>
      <h1 className="font-display text-4xl font-extrabold tracking-tight mb-8">Settings</h1>

      <div className="dossier p-6 space-y-6">
        <div>
          <TextInput
            label="Global match threshold"
            hint="Fallback for subjects without their own threshold. Higher is stricter. 0.40–0.55 is typical for kids."
            inputMode="decimal"
            value={threshold}
            onChange={(e) => setThreshold(e.target.value)}
          />
        </div>

        <div>
          <span className="stamp block text-[11px] text-haze mb-2">Delivery</span>
          <div className="grid gap-2">
            {MODES.map((m) => (
              <button
                key={m.id}
                onClick={() => setMode(m.id)}
                className={
                  "flex items-center justify-between px-4 h-12 border text-left transition-colors " +
                  (mode === m.id
                    ? "border-signal text-bone"
                    : "border-edge text-haze hover:text-bone")
                }
              >
                <span className="font-medium">{m.label}</span>
                <span className="data text-[11px] text-haze">{m.hint}</span>
              </button>
            ))}
          </div>
        </div>

        <div className="flex items-center justify-between pt-2">
          <span className="data text-[11px] text-haze">
            provider <span className="text-signal">{settings.provider}</span>
          </span>
          <Button variant="primary" onClick={save} disabled={saving || !thresholdValid}>
            {saving ? "Saving…" : "Save changes"}
          </Button>
        </div>
      </div>

      <p className="text-haze text-sm mt-6">
        These apply immediately to new sightings — no restart needed. Per-subject
        thresholds (set on a dossier) always win over the global value.
      </p>
    </section>
  );
}
