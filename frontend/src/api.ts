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

// Multipart uploads must not set Content-Type: the browser adds the boundary.
async function upload<T>(path: string, form: FormData): Promise<T> {
  const headers = new Headers();
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const res = await fetch(path, { method: "POST", body: form, headers });
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

// uploadForBlob posts a file and returns the binary response (plus a suggested
// filename), used by the mp4 → mp3 converter.
async function uploadForBlob(
  path: string,
  form: FormData
): Promise<{ blob: Blob; filename: string }> {
  const headers = new Headers();
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const res = await fetch(path, { method: "POST", body: form, headers });
  if (res.status === 401) {
    clearToken();
    throw new ApiError(401, "Не авторизован");
  }
  if (!res.ok) {
    const text = await res.text();
    let message = `Ошибка ${res.status}`;
    try {
      message = JSON.parse(text).error || message;
    } catch {
      // non-JSON error body — keep the generic message
    }
    throw new ApiError(res.status, message);
  }
  return {
    blob: await res.blob(),
    filename: res.headers.get("X-Filename") || "audio.mp3",
  };
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
  templateId: string;
  imageSettings: Record<string, string>;
  videoModel: string;
  videoPrompt: string;
  videoDuration?: number | null;
  videoResolution: string;
  videoAspectRatio: string;
  generateAudio: boolean;
  audioAssetId?: string;
  audioObject?: string;
  status: string;
  error: string;
  attempts: number;
  imageUrl: string;
  videoUrl?: string;
  downloadUrl?: string;
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

export interface MediaAsset {
  id: string;
  kind: string;
  title: string;
  filename: string;
  object: string;
  contentType: string;
  sizeBytes: number;
  durationSeconds: number;
  createdAt: string;
  updatedAt: string;
  url?: string;
}

export interface CreateBatchPayload {
  title: string;
  templateId: string;
  videoModel: string;
  videoPrompt: string;
  videoDuration?: number | null;
  videoResolution?: string;
  videoAspectRatio?: string;
  generateAudio: boolean;
  audioAssetId?: string;
  extraSettings: Record<string, string>;
  firstNameKey: string;
  lastNameKey: string;
  fullNameKey: string;
  names: { firstName: string; lastName: string }[];
}

export interface CampaignDiagnostics {
  duplicateCount: number;
  invalidCapitalization: string[];
}

export interface AdCampaign {
  id: string;
  title: string;
  lifecycle: string;
  nameTextTemplate: string;
  surnameTextTemplate: string;
  templateId: string;
  imageSettings: Record<string, string>;
  nameSettingKey: string;
  videoModel: string;
  videoPrompt: string;
  videoDuration: number | null;
  videoResolution: string;
  videoAspectRatio: string;
  generateAudio: boolean;
  audioAssetId?: string;
  createdAt: string;
  startedAt?: string;
  total: number;
  nameCount: number;
  surnameCount: number;
  done: number;
  failed: number;
  costUsd: number;
  costRub: number;
  vkSettings?: import("./vkSettings").VKSettings;
  vkAdPlanId?: string;
  vkUploadError?: string;
  vkIgnoreFailed?: boolean;
  names?: string[];
  surnames?: string[];
}

export interface AdCampaignItem {
  id: string;
  campaignId: string;
  kind: "name" | "surname";
  value: string;
  status: string;
  error: string;
  attempts: number;
  costUsd: number;
  costRub: number;
  imageDownloadUrl?: string;
  sourceDownloadUrl?: string;
  videoDownloadUrl?: string;
  audienceId?: number;
  audienceName?: string;
  vkAdGroupId?: string;
  vkBannerId?: string;
}

export interface CreateAdCampaignPayload {
  title: string;
  names: string[];
  surnames: string[];
  nameTextTemplate: string;
  surnameTextTemplate: string;
  templateId: string;
  imageSettings: Record<string, string>;
  nameSettingKey: string;
  videoModel: string;
  videoPrompt: string;
  videoDuration: number | null;
  videoResolution: string;
  videoAspectRatio: string;
  generateAudio: boolean;
  audioAssetId: string;
  vkSettings: import("./vkSettings").VKSettings;
}

// ---------- endpoints ----------

export const api = {
  login: (login: string, password: string) =>
    request<{ token: string }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ login, password }),
    }),

  config: () =>
    request<{
      defaultModel: string;
      defaultDuration: number;
      defaultPrompt: string;
      vkAds?: { configured: boolean; accountName?: string };
    }>("/api/config"),

  models: () => request<{ models: VideoModel[]; defaultModel: string }>("/api/models"),

  vkAdsCabinet: () =>
    request<{ configured: boolean; baseUrl?: string; sudo?: string }>("/api/vk-ads/cabinet"),

  vkAdsPads: (targetAction?: string) =>
    request<{
      configured: boolean;
      trees: import("./padsTree").PadNode[];
      packageName?: string;
      defaultPads?: number[];
    }>(`/api/vk-ads/pads${targetAction ? `?targetAction=${encodeURIComponent(targetAction)}` : ""}`),

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

  retryTask: (id: string) =>
    request<{ id: string; status: string }>(`/api/tasks/${id}/retry`, { method: "POST" }),

  retryBatch: (id: string) =>
    request<{ retried: number }>(`/api/batches/${id}/retry`, { method: "POST" }),

  listBatches: () => request<{ batches: Batch[]; usdRubRate: number }>("/api/batches"),

  getBatch: (id: string) =>
    request<{ batch: Batch; usdRubRate: number }>(`/api/batches/${id}`),

  deleteBatch: (id: string) =>
    request<{ deleted: string }>(`/api/batches/${id}`, { method: "DELETE" }),

  createBatchArchive: (id: string) =>
    request<{ downloadUrl: string; filename: string; count: number }>(
      `/api/batches/${id}/archive`,
      { method: "POST" },
    ),

  campaignDefaults: () =>
    request<{
      names: string[];
      surnames: string[];
      nameTextTemplate: string;
      surnameTextTemplate: string;
      diagnostics: { names: CampaignDiagnostics; surnames: CampaignDiagnostics };
      vkSettings: import("./vkSettings").VKSettings;
    }>("/api/ad-campaigns/defaults"),

  createAdCampaign: (payload: CreateAdCampaignPayload) =>
    request<{
      campaign: AdCampaign;
      diagnostics: { names: CampaignDiagnostics; surnames: CampaignDiagnostics };
    }>("/api/ad-campaigns", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  listAdCampaigns: () =>
    request<{ campaigns: AdCampaign[]; usdRubRate: number }>("/api/ad-campaigns"),

  getAdCampaign: (id: string) =>
    request<{ campaign: AdCampaign; usdRubRate: number }>(`/api/ad-campaigns/${id}`),

  listAdCampaignItems: (
    id: string,
    options: { limit?: number; offset?: number; kind?: "name" | "surname"; status?: string } = {},
  ) => {
    const params = new URLSearchParams({
      limit: String(options.limit ?? 50),
      offset: String(options.offset ?? 0),
    });
    if (options.kind) params.set("kind", options.kind);
    if (options.status) params.set("status", options.status);
    return request<{
      items: AdCampaignItem[];
      total: number;
      limit: number;
      offset: number;
      usdRubRate: number;
    }>(`/api/ad-campaigns/${id}/items?${params.toString()}`);
  },

  startAdCampaign: (id: string) =>
    request<{ status: string; queued: number }>(`/api/ad-campaigns/${id}/start`, {
      method: "POST",
    }),

  ignoreAdCampaignFailures: (id: string) =>
    request<{ status: string }>(`/api/ad-campaigns/${id}/ignore-failed`, {
      method: "POST",
    }),

  retryAdCampaign: (id: string) =>
    request<{ retried: number }>(`/api/ad-campaigns/${id}/retry`, { method: "POST" }),

  retryAdCampaignItem: (id: string) =>
    request<{ id: string; status: string }>(`/api/ad-campaigns/items/${id}/retry`, {
      method: "POST",
    }),

  deleteAdCampaign: (id: string) =>
    request<{ deleted: string }>(`/api/ad-campaigns/${id}`, { method: "DELETE" }),

  listAudio: () => request<{ assets: MediaAsset[] }>("/api/media/audio"),

  uploadAudio: (file: File, title?: string) => {
    const form = new FormData();
    form.append("file", file);
    if (title) form.append("title", title);
    return upload<{ asset: MediaAsset }>("/api/media/audio", form);
  },

  deleteAudio: (id: string) =>
    request<{ deleted: string }>(`/api/media/audio/${id}`, { method: "DELETE" }),

  extractAudio: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return uploadForBlob("/api/media/extract-audio", form);
  },
};
