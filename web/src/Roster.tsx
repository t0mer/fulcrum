import { useEffect, useState } from "react";
import { api, ApiError, type Subject } from "./api";
import { Button, Modal, Spinner, TextInput, useToast } from "./ui";

function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 32);
}

export function Roster({ onOpen }: { onOpen: (id: number) => void }) {
  const [subjects, setSubjects] = useState<Subject[] | null>(null);
  const [creating, setCreating] = useState(false);
  const toast = useToast();

  const load = () =>
    api
      .listSubjects()
      .then(setSubjects)
      .catch((e) => toast(e.message, "err"));

  useEffect(() => {
    load();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <section>
      <div className="flex items-end justify-between mb-8 gap-4 flex-wrap">
        <div>
          <p className="stamp text-[11px] text-haze mb-2">under watch</p>
          <h1 className="font-display text-4xl sm:text-5xl font-extrabold tracking-tight">
            {subjects === null ? "—" : subjects.length}{" "}
            <span className="text-haze font-semibold text-2xl align-middle">
              {subjects?.length === 1 ? "subject" : "subjects"}
            </span>
          </h1>
        </div>
        <Button variant="primary" onClick={() => setCreating(true)}>
          + Enroll subject
        </Button>
      </div>

      {subjects === null && <Spinner label="Loading registry…" />}

      {subjects?.length === 0 && (
        <div className="dossier p-10 text-center">
          <p className="stamp text-haze text-sm mb-2">registry empty</p>
          <p className="text-haze max-w-sm mx-auto">
            Enroll a child to start watching for them. You'll add a few recent,
            varied reference photos so Fulcrum learns their face.
          </p>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {subjects?.map((s) => (
          <button
            key={s.id}
            onClick={() => onOpen(s.id)}
            className="dossier p-5 text-left rise hover:border-signal transition-colors group"
          >
            <div className="flex items-center justify-between mb-6">
              <span className="data text-[11px] text-haze">
                #{String(s.id).padStart(3, "0")}
              </span>
              <span className="data text-[11px] text-haze">
                thr {(s.threshold ?? 0) > 0 ? s.threshold?.toFixed(2) : "auto"}
              </span>
            </div>
            <h3 className="text-2xl font-semibold mb-1 group-hover:text-signal transition-colors">
              {s.name}
            </h3>
            <p className="data text-sm text-signal mb-4">@{s.slug}</p>
            <p className="data text-[11px] text-haze">
              {s.face_count ?? 0} reference {s.face_count === 1 ? "photo" : "photos"}
            </p>
          </button>
        ))}
      </div>

      {creating && (
        <CreateSubject
          onClose={() => setCreating(false)}
          onCreated={() => {
            setCreating(false);
            load();
            toast("Subject enrolled");
          }}
        />
      )}
    </section>
  );
}

function CreateSubject({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  const effectiveSlug = slugTouched ? slug : slugify(name);

  const submit = async () => {
    setBusy(true);
    try {
      await api.createSubject({ name: name.trim(), slug: effectiveSlug });
      onCreated();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Could not enroll subject", "err");
      setBusy(false);
    }
  };

  return (
    <Modal title="Enroll subject" onClose={onClose}>
      <div className="space-y-4">
        <TextInput
          label="Display name"
          placeholder="e.g. יעל"
          value={name}
          autoFocus
          onChange={(e) => setName(e.target.value)}
        />
        <TextInput
          label="Call sign (folder slug)"
          hint="Lowercase latin, stable on disk. a–z, 0–9, hyphen."
          placeholder="yael"
          value={effectiveSlug}
          onChange={(e) => {
            setSlugTouched(true);
            setSlug(e.target.value);
          }}
        />
        <div className="flex justify-end gap-2 pt-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button
            variant="primary"
            disabled={busy || !name.trim() || !/^[a-z0-9-]{1,32}$/.test(effectiveSlug)}
            onClick={submit}
          >
            {busy ? "Enrolling…" : "Enroll"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
