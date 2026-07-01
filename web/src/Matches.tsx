import { useEffect, useState } from "react";
import { api, ApiError, type Match } from "./api";
import { AuthImage, Button, Spinner, useToast } from "./ui";

type Filter = "all" | "unreviewed" | "confirmed" | "rejected";

export function Matches({ onOpenSubject }: { onOpenSubject: (id: number) => void }) {
  const [matches, setMatches] = useState<Match[] | null>(null);
  const [filter, setFilter] = useState<Filter>("all");
  const toast = useToast();

  const load = (f: Filter) =>
    api
      .listMatches(f === "all" ? undefined : { reviewed: f })
      .then(setMatches)
      .catch((e) => toast(e.message, "err"));

  useEffect(() => {
    load(filter);
  }, [filter]); // eslint-disable-line react-hooks/exhaustive-deps

  const review = async (m: Match, decision: "confirm" | "reject") => {
    try {
      const res = await api.reviewMatch(m.id, decision);
      const msg =
        decision === "reject"
          ? "Match rejected"
          : res.reinforced
            ? "Confirmed · added as a reference"
            : "Match confirmed";
      toast(msg);
      await load(filter);
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Review failed", "err");
    }
  };

  return (
    <section>
      <div className="flex items-end justify-between mb-8 gap-4 flex-wrap">
        <div>
          <p className="stamp text-[11px] text-haze mb-2">watch log</p>
          <h1 className="font-display text-4xl font-extrabold tracking-tight">
            {matches === null ? "—" : matches.length}{" "}
            <span className="text-haze font-semibold text-2xl align-middle">
              {matches?.length === 1 ? "sighting" : "sightings"}
            </span>
          </h1>
        </div>
        <div className="flex gap-1">
          {(["all", "unreviewed", "confirmed", "rejected"] as Filter[]).map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={
                "stamp text-[10px] px-3 h-8 border transition-colors " +
                (filter === f ? "border-signal text-signal" : "border-edge text-haze hover:text-bone")
              }
            >
              {f}
            </button>
          ))}
        </div>
      </div>

      {matches === null && <Spinner label="Loading sightings…" />}

      {matches?.length === 0 && (
        <div className="dossier p-10 text-center">
          <p className="stamp text-haze text-sm mb-2">nothing yet</p>
          <p className="text-haze max-w-sm mx-auto">
            When a watched group shares a photo with one of your kids, it appears here.
          </p>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {matches?.map((m) => (
          <article key={m.id} className="dossier overflow-hidden rise flex flex-col">
            {m.stored_path ? (
              <AuthImage
                url={api.matchImageURL(m.id)}
                alt={`match for ${m.subject_name}`}
                className="w-full aspect-square object-cover"
              />
            ) : (
              <div className="w-full aspect-square grid place-items-center text-haze data text-[11px]">
                forwarded · not stored
              </div>
            )}
            <div className="p-4 flex-1 flex flex-col gap-2">
              <div className="flex items-center justify-between">
                <button
                  onClick={() => onOpenSubject(m.subject_id)}
                  className="text-lg font-semibold hover:text-signal transition-colors"
                >
                  {m.subject_name}
                </button>
                <span className="data text-signal text-sm">{m.similarity.toFixed(2)}</span>
              </div>
              <p className="data text-[11px] text-haze">
                from {m.source_group} · {new Date(m.created_at).toLocaleString()}
              </p>
              <div className="flex items-center gap-2 mt-2">
                {m.reviewed === "unreviewed" ? (
                  <>
                    <Button variant="primary" className="h-8 px-3 text-xs" onClick={() => review(m, "confirm")}>
                      Confirm
                    </Button>
                    <Button variant="danger" className="h-8 px-3 text-xs" onClick={() => review(m, "reject")}>
                      Reject
                    </Button>
                  </>
                ) : (
                  <span
                    className={
                      "stamp text-[10px] " +
                      (m.reviewed === "confirmed"
                        ? "text-[color:var(--ready)]"
                        : "text-red-400")
                    }
                  >
                    {m.reviewed}
                  </span>
                )}
                {m.forwarded && <span className="stamp text-[10px] text-haze ml-auto">forwarded</span>}
              </div>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
