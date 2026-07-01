import { useEffect, useRef, useState } from "react";
import { api, ApiError, type FaceCandidate, type SubjectDetail, type Tuning } from "./api";
import { AuthImage, Button, Modal, Spinner, TextInput, useToast } from "./ui";

export function SubjectPanel({ id, onBack }: { id: number; onBack: () => void }) {
  const [subject, setSubject] = useState<SubjectDetail | null>(null);
  const [pending, setPending] = useState<{ file: File; candidates: FaceCandidate[] } | null>(null);
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  const load = () =>
    api
      .getSubject(id)
      .then(setSubject)
      .catch((e) => toast(e.message, "err"));

  useEffect(() => {
    load();
  }, [id]); // eslint-disable-line react-hooks/exhaustive-deps

  const upload = async (file: File, faceIndex?: number) => {
    setBusy(true);
    try {
      const res = await api.uploadFace(id, file, faceIndex);
      if ("candidates" in res) {
        setPending({ file, candidates: res.candidates });
      } else {
        setPending(null);
        toast("Reference photo added");
        await load();
      }
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 422
          ? "No face found in that photo"
          : e instanceof ApiError
            ? e.message
            : "Upload failed";
      toast(msg, "err");
    } finally {
      setBusy(false);
    }
  };

  const removeFace = async (faceId: number) => {
    try {
      await api.deleteFace(id, faceId);
      await load();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Could not delete", "err");
    }
  };

  const reembed = async () => {
    setBusy(true);
    try {
      const { embeddings } = await api.reembedSubject(id);
      toast(`Recomputed ${embeddings} embedding${embeddings === 1 ? "" : "s"}`);
      await load();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Re-embed failed", "err");
    } finally {
      setBusy(false);
    }
  };

  const removeSubject = async () => {
    if (!confirm(`Delete ${subject?.name} and all their reference photos?`)) return;
    try {
      await api.deleteSubject(id);
      toast("Subject removed");
      onBack();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Could not delete", "err");
    }
  };

  if (!subject) return <Spinner label="Opening dossier…" />;

  return (
    <section>
      <button onClick={onBack} className="stamp text-[11px] text-haze hover:text-signal mb-6">
        ← registry
      </button>

      <div className="dossier p-6 mb-6">
        <div className="flex items-start justify-between gap-4 flex-wrap">
          <div>
            <div className="data text-[11px] text-haze mb-2">
              #{String(subject.id).padStart(3, "0")}
            </div>
            <h1 className="font-display text-4xl font-extrabold tracking-tight">{subject.name}</h1>
            <p className="data text-signal mt-1">@{subject.slug}</p>
          </div>
          <div className="text-right">
            <div className="stamp text-[11px] text-haze mb-1">match threshold</div>
            <button onClick={() => setEditing(true)} className="data text-2xl hover:text-signal">
              {subject.threshold ? subject.threshold.toFixed(2) : "auto"}
            </button>
          </div>
        </div>

        <div className="flex gap-2 mt-6 flex-wrap">
          <Button onClick={reembed} disabled={busy}>
            Recompute embeddings
          </Button>
          <Button variant="danger" onClick={removeSubject}>
            Delete subject
          </Button>
        </div>
      </div>

      <TuningCard
        id={id}
        onApplied={async () => {
          await load();
          toast("Threshold applied");
        }}
      />

      <Dropzone busy={busy} onFile={(f) => upload(f)} />

      <div className="flex items-baseline justify-between mt-8 mb-4">
        <h2 className="stamp text-sm text-haze">reference photos</h2>
        <span className="data text-[11px] text-haze">{subject.faces.length} enrolled</span>
      </div>

      {subject.faces.length === 0 ? (
        <p className="text-haze text-sm">
          None yet. Add 5–15 varied, recent photos for reliable recognition.
        </p>
      ) : (
        <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-3">
          {subject.faces.map((f) => (
            <div key={f.id} className="relative group aspect-square overflow-hidden dossier">
              <AuthImage
                url={`/api/subjects/${id}/faces/${f.id}/image`}
                alt="reference"
                className="h-full w-full object-cover"
              />
              <button
                onClick={() => removeFace(f.id)}
                className="absolute inset-0 grid place-items-center bg-black/70 opacity-0
                           group-hover:opacity-100 transition-opacity stamp text-[11px] text-red-400"
              >
                remove
              </button>
            </div>
          ))}
        </div>
      )}

      {pending && (
        <CandidatePicker
          file={pending.file}
          candidates={pending.candidates}
          busy={busy}
          onPick={(idx) => upload(pending.file, idx)}
          onClose={() => setPending(null)}
        />
      )}

      {editing && (
        <ThresholdEditor
          current={subject.threshold ?? null}
          onClose={() => setEditing(false)}
          onSave={async (value) => {
            try {
              await api.updateSubject(id, value === null ? { clear_threshold: true } : { threshold: value });
              setEditing(false);
              await load();
              toast("Threshold updated");
            } catch (e) {
              toast(e instanceof ApiError ? e.message : "Could not update", "err");
            }
          }}
        />
      )}
    </section>
  );
}

function TuningCard({ id, onApplied }: { id: number; onApplied: () => void }) {
  const [tuning, setTuning] = useState<Tuning | null>(null);
  const [applying, setApplying] = useState(false);
  const toast = useToast();

  useEffect(() => {
    api
      .getTuning(id)
      .then(setTuning)
      .catch(() => {});
  }, [id]);

  if (!tuning) return null;
  const { stats, suggestion } = tuning;
  // Nothing to say until there's some review history.
  if (stats.confirmed_count === 0 && stats.rejected_count === 0) return null;

  const apply = async () => {
    setApplying(true);
    try {
      await api.updateSubject(id, { threshold: suggestion.threshold });
      onApplied();
    } catch {
      toast("Could not apply threshold", "err");
    } finally {
      setApplying(false);
    }
  };

  return (
    <div className="dossier p-5 mt-6">
      <div className="flex items-center justify-between mb-3">
        <span className="stamp text-[11px] text-haze">threshold tuning</span>
        <span className="data text-[11px] text-haze">
          {stats.confirmed_count} confirmed · {stats.rejected_count} rejected
        </span>
      </div>
      <div className="flex flex-wrap items-end gap-x-8 gap-y-3">
        <Stat label="lowest confirmed" value={stats.min_confirmed} />
        <Stat label="highest rejected" value={stats.max_rejected} />
        {suggestion.has_suggestion && (
          <div>
            <div className="stamp text-[11px] text-haze mb-1">suggested</div>
            <div className="data text-2xl text-signal">{suggestion.threshold.toFixed(2)}</div>
          </div>
        )}
        {suggestion.has_suggestion && (
          <Button className="ml-auto" onClick={apply} disabled={applying}>
            {applying ? "Applying…" : "Apply suggestion"}
          </Button>
        )}
      </div>
      {suggestion.overlap && (
        <p className="text-[11px] text-red-400 data mt-3">
          A rejected face scored above your lowest confirmed one — these can't be
          cleanly separated. Add more varied reference photos.
        </p>
      )}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div className="stamp text-[11px] text-haze mb-1">{label}</div>
      <div className="data text-2xl">{value > 0 ? value.toFixed(2) : "—"}</div>
    </div>
  );
}

function Dropzone({ busy, onFile }: { busy: boolean; onFile: (f: File) => void }) {
  const [over, setOver] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        const f = e.dataTransfer.files[0];
        if (f) onFile(f);
      }}
      className={
        "dossier p-8 text-center cursor-pointer transition-colors " +
        (over ? "border-signal bg-[color:var(--signal-soft)]" : "")
      }
      onClick={() => input.current?.click()}
    >
      <input
        ref={input}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) onFile(f);
          e.target.value = "";
        }}
      />
      {busy ? (
        <Spinner label="Detecting face…" />
      ) : (
        <>
          <p className="stamp text-sm text-signal mb-1">drop a photo</p>
          <p className="text-haze text-sm">or click to browse · JPG, PNG, WebP</p>
        </>
      )}
    </div>
  );
}

function CandidatePicker({
  file,
  candidates,
  busy,
  onPick,
  onClose,
}: {
  file: File;
  candidates: FaceCandidate[];
  busy: boolean;
  onPick: (index: number) => void;
  onClose: () => void;
}) {
  const [dims, setDims] = useState<{ w: number; h: number } | null>(null);
  const url = URL.createObjectURL(file);
  useEffect(() => () => URL.revokeObjectURL(url), [url]);

  return (
    <Modal title="Which face is theirs?" onClose={onClose}>
      <p className="text-haze text-sm mb-4">
        Several faces were found. Tap the one to enroll.
      </p>
      <div className="relative select-none">
        <img
          src={url}
          alt="upload"
          className="w-full"
          onLoad={(e) => setDims({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
        />
        {dims &&
          candidates.map((c) => (
            <button
              key={c.index}
              onClick={() => onPick(c.index)}
              disabled={busy}
              title={`score ${c.det_score.toFixed(2)}`}
              className="absolute border-2 border-signal hover:bg-[color:var(--signal-soft)]"
              style={{
                left: `${(c.bbox[0] / dims.w) * 100}%`,
                top: `${(c.bbox[1] / dims.h) * 100}%`,
                width: `${((c.bbox[2] - c.bbox[0]) / dims.w) * 100}%`,
                height: `${((c.bbox[3] - c.bbox[1]) / dims.h) * 100}%`,
              }}
            >
              <span className="absolute -top-5 left-0 data text-[10px] text-signal">
                {c.det_score.toFixed(2)}
              </span>
            </button>
          ))}
      </div>
    </Modal>
  );
}

function ThresholdEditor({
  current,
  onClose,
  onSave,
}: {
  current: number | null;
  onClose: () => void;
  onSave: (value: number | null) => void;
}) {
  const [value, setValue] = useState(current ? String(current) : "");

  return (
    <Modal title="Match threshold" onClose={onClose}>
      <p className="text-haze text-sm mb-4">
        Higher is stricter (fewer false matches). Leave blank to use the global
        default. Kids and siblings often need tuning — 0.40–0.55 is typical.
      </p>
      <TextInput
        label="Threshold (0–1)"
        inputMode="decimal"
        placeholder="auto"
        value={value}
        autoFocus
        onChange={(e) => setValue(e.target.value)}
      />
      <div className="flex justify-between gap-2 pt-5">
        <Button variant="danger" onClick={() => onSave(null)}>
          Reset to auto
        </Button>
        <div className="flex gap-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button
            variant="primary"
            disabled={value !== "" && !(Number(value) > 0 && Number(value) < 1)}
            onClick={() => onSave(value === "" ? null : Number(value))}
          >
            Save
          </Button>
        </div>
      </div>
    </Modal>
  );
}
