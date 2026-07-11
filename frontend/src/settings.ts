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
  firstNameKey: string;
  lastNameKey: string;
  fullNameKey: string;
  extraSettingsText: string; // JSON object as text
  title: string;
}

const KEY = "nc_settings";

export const DEFAULT_SETTINGS: UiSettings = {
  namesText: "",
  templateId: "",
  videoModel: "",
  videoPrompt: "",
  videoDuration: "",
  videoResolution: "",
  videoAspectRatio: "",
  firstNameKey: "firstName",
  lastNameKey: "lastName",
  fullNameKey: "name",
  extraSettingsText: "{}",
  title: "",
};

export function loadSettings(): UiSettings {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return { ...DEFAULT_SETTINGS };
    return { ...DEFAULT_SETTINGS, ...JSON.parse(raw) };
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
