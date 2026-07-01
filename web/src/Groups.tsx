import { useEffect, useState } from "react";
import { api, ApiError, type Group, type ProviderStatus } from "./api";
import { Button, Spinner, useToast } from "./ui";

export function Groups() {
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [provider, setProvider] = useState<ProviderStatus | null>(null);
  const [testing, setTesting] = useState(false);
  const toast = useToast();

  const load = () =>
    api
      .listGroups()
      .then(setGroups)
      .catch((e) => toast(e.message, "err"));

  useEffect(() => {
    load();
    api.getProvider().then(setProvider).catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const setMonitored = async (g: Group, monitored: boolean) => {
    try {
      await api.updateGroup(g.id, { monitored });
      await load();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Update failed", "err");
    }
  };

  const setDestination = async (g: Group) => {
    try {
      await api.updateGroup(g.id, { is_destination: !g.is_destination });
      await load();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Update failed", "err");
    }
  };

  const test = async () => {
    setTesting(true);
    try {
      const res = await api.testProvider();
      toast(res.ok ? `Connected · ${res.groups} groups` : `Failed: ${res.error}`, res.ok ? "ok" : "err");
      await load();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Test failed", "err");
    } finally {
      setTesting(false);
    }
  };

  return (
    <section>
      <div className="flex items-end justify-between mb-8 gap-4 flex-wrap">
        <div>
          <p className="stamp text-[11px] text-haze mb-2">channels</p>
          <h1 className="font-display text-4xl font-extrabold tracking-tight">Groups</h1>
          {provider && (
            <p className="data text-[11px] text-haze mt-2">
              provider <span className="text-signal">{provider.name}</span>
            </p>
          )}
        </div>
        <Button onClick={test} disabled={testing}>
          {testing ? "Testing…" : "Refresh from provider"}
        </Button>
      </div>

      {groups === null && <Spinner label="Loading channels…" />}

      {groups?.length === 0 && (
        <div className="dossier p-10 text-center">
          <p className="stamp text-haze text-sm mb-2">no channels</p>
          <p className="text-haze max-w-sm mx-auto">
            Connect your WhatsApp gateway, then refresh to pull in the groups you've joined.
          </p>
        </div>
      )}

      {groups && groups.length > 0 && (
        <div className="dossier divide-y divide-edge">
          {groups.map((g) => (
            <div key={g.id} className="flex items-center justify-between gap-4 p-4 flex-wrap">
              <div className="min-w-0">
                <p className="font-semibold truncate">{g.name}</p>
                <p className="data text-[11px] text-haze truncate">{g.provider_group_id}</p>
              </div>
              <div className="flex items-center gap-2">
                <Toggle
                  active={g.monitored}
                  onClick={() => setMonitored(g, !g.monitored)}
                  label={g.monitored ? "watching" : "watch"}
                />
                <button
                  onClick={() => setDestination(g)}
                  className={
                    "stamp text-[11px] px-3 h-8 border transition-colors " +
                    (g.is_destination
                      ? "border-[color:var(--ready)] text-[color:var(--ready)]"
                      : "border-edge text-haze hover:text-bone")
                  }
                >
                  {g.is_destination ? "destination ✓" : "set destination"}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      <p className="text-haze text-sm mt-6 max-w-lg">
        Watched groups are scanned for your kids. Matches are forwarded to the single
        destination group (if forwarding is on). The destination need not be watched.
      </p>
    </section>
  );
}

function Toggle({
  active,
  onClick,
  label,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      onClick={onClick}
      className={
        "stamp text-[11px] px-3 h-8 border transition-colors " +
        (active ? "border-signal text-signal" : "border-edge text-haze hover:text-bone")
      }
    >
      {label}
    </button>
  );
}
