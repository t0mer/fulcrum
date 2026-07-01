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

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? res.statusText);
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
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

  reembedAll: () => req<{ embeddings: number }>("/subjects/reembed", { method: "POST" }),

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
