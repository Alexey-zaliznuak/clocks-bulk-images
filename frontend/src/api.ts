export const TOKEN_KEY = "nc_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Content-Type", "application/json");
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const res = await fetch(path, { ...options, headers });
  if (res.status === 401) {
    clearToken();
    throw new ApiError(401, "Не авторизован");
  }
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new ApiError(res.status, data.error || `Ошибка ${res.status}`);
  }
  return data as T;
}

// ---------- types ----------

export interface VideoModel {
  id: string;
  name: string;
  supported_resolutions?: string[];
  supported_aspect_ratios?: string[];
  supported_durations?: number[];
}

export interface Task {
  id: string;
  batchId: string;
  firstName: string;
  lastName: string;
  status: string;
  error: string;
  imageUrl: string;
  videoUrl?: string;
  videoObject: string;
  costUsd: number;
  costRub: number;
  createdAt: string;
  updatedAt: string;
}

export interface Batch {
  id: string;
  title: string;
  templateId: string;
  videoModel: string;
  createdAt: string;
  total: number;
  done: number;
  failed: number;
  costUsd: number;
  costRub: number;
}

export interface CreateBatchPayload {
  title: string;
  templateId: string;
  videoModel: string;
  videoPrompt: string;
  videoDuration?: number | null;
  videoResolution?: string;
  videoAspectRatio?: string;
  extraSettings: Record<string, string>;
  firstNameKey: string;
  lastNameKey: string;
  fullNameKey: string;
  names: { firstName: string; lastName: string }[];
}

// ---------- endpoints ----------

export const api = {
  login: (login: string, password: string) =>
    request<{ token: string }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ login, password }),
    }),

  config: () => request<{ defaultModel: string; defaultPrompt: string }>("/api/config"),

  models: () => request<{ models: VideoModel[]; defaultModel: string }>("/api/models"),

  createBatch: (payload: CreateBatchPayload) =>
    request<{ batchId: string; count: number }>("/api/tasks/batch", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  listTasks: (batchId?: string, limit = 500) => {
    const params = new URLSearchParams();
    if (batchId) params.set("batch_id", batchId);
    params.set("limit", String(limit));
    return request<{ tasks: Task[]; usdRubRate: number }>(`/api/tasks?${params.toString()}`);
  },

  listBatches: () => request<{ batches: Batch[]; usdRubRate: number }>("/api/batches"),

  deleteBatch: (id: string) =>
    request<{ deleted: string }>(`/api/batches/${id}`, { method: "DELETE" }),
};
