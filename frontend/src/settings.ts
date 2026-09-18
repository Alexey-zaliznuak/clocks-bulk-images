// Persisted UI settings — everything the user configures on the Create page is
// stored in localStorage so it survives reloads (per the requirements).

export interface UiSettings {
  namesText: string;
  templateId: string;
  videoModel: string;
  videoPrompt: string;
  videoDuration: string; // kept as string for the input; parsed on submit
  videoResolution: string;
  videoAspectRatio: string;
  // Ask the model for a soundtrack instead of muxing an mp3 in. Off by default:
  // audio-capable generations cost noticeably more.
  generateAudio: boolean;
  audioAssetId: string;
  firstNameKey: string;
  lastNameKey: string;
  fullNameKey: string;
  extraSettingsText: string; // JSON object as text
  title: string;
}

const KEY = "nc_settings";
const VIDEO_MODEL_DEFAULT_KEY = "nc_video_model_default_v2";
export const DEFAULT_VIDEO_MODEL = "google/veo-3.1-lite";

export const DEFAULT_SETTINGS: UiSettings = {
  namesText: "",
  templateId: "",
  videoModel: DEFAULT_VIDEO_MODEL,
  videoPrompt: "",
  videoDuration: "4",
  videoResolution: "",
  videoAspectRatio: "",
  generateAudio: false,
  audioAssetId: "",
  firstNameKey: "firstName",
  lastNameKey: "lastName",
  fullNameKey: "name",
  extraSettingsText: "{}",
  title: "",
};

export function loadSettings(): UiSettings {
  try {
    const raw = localStorage.getItem(KEY);
    const settings = raw
      ? { ...DEFAULT_SETTINGS, ...JSON.parse(raw) }
      : { ...DEFAULT_SETTINGS };
    if (localStorage.getItem(VIDEO_MODEL_DEFAULT_KEY) !== DEFAULT_VIDEO_MODEL) {
      settings.videoModel = DEFAULT_VIDEO_MODEL;
      localStorage.setItem(VIDEO_MODEL_DEFAULT_KEY, DEFAULT_VIDEO_MODEL);
    }
    return settings;
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

export function saveSettings(s: UiSettings) {
  localStorage.setItem(KEY, JSON.stringify(s));
}

// Parse the free-form textarea into first/last name pairs.
// Each non-empty line: first token = first name, the rest = last name.
export function parseNames(text: string): { firstName: string; lastName: string }[] {
  return text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .map((line) => {
      const parts = line.split(/\s+/);
      const firstName = parts.shift() || "";
      const lastName = parts.join(" ");
      return { firstName, lastName };
    });
}
