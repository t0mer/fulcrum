// Typed client for the Fulcrum REST API. The UI is a client of /api/v1-style
// endpoints (see CLAUDE.md §16); every action here maps to one endpoint.

export interface Subject {
  id: number;
  name: string;
  slug: string;
  threshold?: number | null;
  created_at: string;
  face_count?: number;
}

export interface Face {
  id: number;
  subject_id: number;
  source_path: string;
  added_at: string;
}

export interface SubjectDetail extends Subject {
  faces: Face[];
}

export interface FaceCandidate {
  index: number;
  bbox: number[];
  det_score: number;
}

export interface Group {
  id: number;
  provider_group_id: string;
  name: string;
  monitored: boolean;
  is_destination: boolean;
  last_seen?: string | null;
}

export interface Match {
  id: number;
  message_id: string;
  subject_id: number;
  subject_name: string;
  subject_slug: string;
  similarity: number;
  source_group: string;
  stored_path: string;
  forwarded: boolean;
  reviewed: "unreviewed" | "confirmed" | "rejected";
  created_at: string;
}

export interface ProviderStatus {
  name: string;
  connected: boolean;
}

export interface Settings {
  global_threshold: number;
  sink_mode: "storage-only" | "forward-only" | "both";
  provider: string;
}

export interface Tuning {
  stats: {
    confirmed_count: number;
    rejected_count: number;
    min_confirmed: number;
    max_rejected: number;
  };
  suggestion: {
    threshold: number;
    has_suggestion: boolean;
    overlap: boolean;
  };
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

const TOKEN_KEY = "fulcrum-api-token";

export const auth = {
  get: () => localStorage.getItem(TOKEN_KEY) ?? "",
  set: (t: string) => localStorage.setItem(TOKEN_KEY, t),
  clear: () => localStorage.removeItem(TOKEN_KEY),
};

function authHeaders(): Record<string, string> {
  const t = auth.get();
  return t ? { "X-API-Token": t } : {};
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    headers: { ...(init?.headers as Record<string, string>), ...authHeaders() },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? res.statusText);
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
}

// fetchImageURL loads an image with the auth header (when a token is set) and
// returns an object URL, since <img> tags can't send custom headers. With no
// token it returns the direct URL so the browser loads it natively.
export async function fetchImageURL(url: string): Promise<string> {
  if (!auth.get()) return url;
  const res = await fetch(url, { headers: authHeaders() });
  if (!res.ok) throw new ApiError(res.status, res.statusText);
  return URL.createObjectURL(await res.blob());
}

export async function getAuthInfo(): Promise<{ auth_required: boolean }> {
  const res = await fetch("/api/authinfo");
  return res.json();
}

export const api = {
  listSubjects: () => req<Subject[]>("/subjects/"),

  getSubject: (id: number) => req<SubjectDetail>(`/subjects/${id}/`),

  createSubject: (body: { name: string; slug: string; threshold?: number | null }) =>
    req<Subject>("/subjects/", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),

  updateSubject: (
    id: number,
    body: { name?: string; threshold?: number | null; clear_threshold?: boolean },
  ) =>
    req<Subject>(`/subjects/${id}/`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),

  deleteSubject: (id: number) => req<Subject>(`/subjects/${id}/`, { method: "DELETE" }),

  deleteFace: (subjectId: number, faceId: number) =>
    req<{ deleted: number }>(`/subjects/${subjectId}/faces/${faceId}`, { method: "DELETE" }),

  reembedSubject: (id: number) =>
    req<{ embeddings: number }>(`/subjects/${id}/reembed`, { method: "POST" }),

  getTuning: (id: number) => req<Tuning>(`/subjects/${id}/tuning`),

  reembedAll: () => req<{ embeddings: number }>("/subjects/reembed", { method: "POST" }),

  listGroups: () => req<Group[]>("/groups/"),

  updateGroup: (id: number, body: { monitored?: boolean; is_destination?: boolean }) =>
    req<{ status: string }>(`/groups/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),

  listMatches: (filter?: { subject_id?: number; reviewed?: string }) => {
    const p = new URLSearchParams();
    if (filter?.subject_id) p.set("subject_id", String(filter.subject_id));
    if (filter?.reviewed) p.set("reviewed", filter.reviewed);
    const qs = p.toString();
    return req<Match[]>(`/matches/${qs ? `?${qs}` : ""}`);
  },

  reviewMatch: (id: number, decision: "confirm" | "reject") =>
    req<{ reviewed: string; reinforced?: boolean }>(`/matches/${id}/review`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ decision }),
    }),

  matchImageURL: (id: number) => `/api/matches/${id}/image`,

  getProvider: () => req<ProviderStatus>("/provider"),

  testProvider: () => req<{ ok: boolean; groups?: number; error?: string }>("/provider/test", {
    method: "POST",
  }),

  getSettings: () => req<Settings>("/settings"),

  updateSettings: (body: { global_threshold?: number; sink_mode?: string }) =>
    req<Settings>("/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),

  // Upload returns the created face, or a 300 with candidates when several
  // faces are found and the caller must choose one.
  uploadFace: async (
    subjectId: number,
    file: File,
    faceIndex?: number,
  ): Promise<{ face: Face } | { candidates: FaceCandidate[] }> => {
    const form = new FormData();
    form.append("file", file);
    if (faceIndex !== undefined) form.append("face_index", String(faceIndex));
    const res = await fetch(`/api/subjects/${subjectId}/faces`, {
      method: "POST",
      headers: authHeaders(),
      body: form,
    });
    if (res.status === 300) {
      const body = await res.json();
      return { candidates: body.candidates as FaceCandidate[] };
    }
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new ApiError(res.status, body.error ?? res.statusText);
    }
    return { face: (await res.json()) as Face };
  },
};
