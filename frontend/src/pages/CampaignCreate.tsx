import { useEffect, useMemo, useState, type DragEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, type CampaignDiagnostics, type MediaAsset, type VideoModel } from "../api";
import {
  applyNamePreview,
  hasNamePlaceholder,
  normalizeCampaignList,
  type NormalizedList,
} from "../campaignValidation";
import ConfirmDialog from "../components/ConfirmDialog";
import { formatDuration } from "../format";
import { DEFAULT_VIDEO_MODEL } from "../settings";

type Tab = "names" | "surnames";

interface CampaignFormSettings {
  templateId: string;
  nameSettingKey: string;
  imageSettingsText: string;
  videoModel: string;
  videoPrompt: string;
  videoDuration: string;
  videoResolution: string;
  videoAspectRatio: string;
  generateAudio: boolean;
  audioAssetId: string;
}

const STORAGE_KEY = "nc_campaign_settings";
const VIDEO_MODEL_DEFAULT_KEY = "nc_campaign_video_model_default_v2";
const initialSettings: CampaignFormSettings = {
  templateId: "",
  nameSettingKey: "name",
  imageSettingsText: "{}",
  videoModel: DEFAULT_VIDEO_MODEL,
  videoPrompt: "",
  videoDuration: "4",
  videoResolution: "",
  videoAspectRatio: "",
  generateAudio: false,
  audioAssetId: "",
};

function loadCampaignSettings(): CampaignFormSettings {
  try {
    const settings = {
      ...initialSettings,
      ...JSON.parse(localStorage.getItem(STORAGE_KEY) || "{}"),
    } as CampaignFormSettings;
    if (localStorage.getItem(VIDEO_MODEL_DEFAULT_KEY) !== DEFAULT_VIDEO_MODEL) {
      settings.videoModel = DEFAULT_VIDEO_MODEL;
      localStorage.setItem(VIDEO_MODEL_DEFAULT_KEY, DEFAULT_VIDEO_MODEL);
    }
    return settings;
  } catch {
    return { ...initialSettings };
  }
}

export default function CampaignCreate() {
  const navigate = useNavigate();
  const [title, setTitle] = useState("");
  const [activeTab, setActiveTab] = useState<Tab>("names");
  const [namesText, setNamesText] = useState("");
  const [surnamesText, setSurnamesText] = useState("");
  const [nameTextTemplate, setNameTextTemplate] = useState("");
  const [surnameTextTemplate, setSurnameTextTemplate] = useState("");
  const [defaultNames, setDefaultNames] = useState<string[]>([]);
  const [defaultSurnames, setDefaultSurnames] = useState<string[]>([]);
  const [defaultNameDiagnostics, setDefaultNameDiagnostics] = useState<CampaignDiagnostics | null>(null);
  const [defaultSurnameDiagnostics, setDefaultSurnameDiagnostics] = useState<CampaignDiagnostics | null>(null);
  const [nameSource, setNameSource] = useState("Дефолтный список");
  const [surnameSource, setSurnameSource] = useState("Дефолтный список");
  const [settings, setSettings] = useState<CampaignFormSettings>(loadCampaignSettings);
  const [models, setModels] = useState<VideoModel[]>([]);
  const [modelsError, setModelsError] = useState("");
  const [audio, setAudio] = useState<MediaAsset[]>([]);
  const [audioError, setAudioError] = useState("");
  const [loadingDefaults, setLoadingDefaults] = useState(true);
  const [defaultsError, setDefaultsError] = useState("");
  const [audioWarnOpen, setAudioWarnOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  }, [settings]);

  useEffect(() => {
    api.campaignDefaults()
      .then((res) => {
        setDefaultNames(res.names);
        setDefaultSurnames(res.surnames);
        setNamesText(res.names.join("\n"));
        setSurnamesText(res.surnames.join("\n"));
        setNameTextTemplate(res.nameTextTemplate);
        setSurnameTextTemplate(res.surnameTextTemplate);
        setDefaultNameDiagnostics(res.diagnostics.names);
        setDefaultSurnameDiagnostics(res.diagnostics.surnames);
      })
      .catch((e) => setDefaultsError(e instanceof Error ? e.message : "Не удалось загрузить списки"))
      .finally(() => setLoadingDefaults(false));

    api.config()
      .then((cfg) => setSettings((s) => ({
        ...s,
        videoModel: s.videoModel || cfg.defaultModel,
        videoPrompt: s.videoPrompt || cfg.defaultPrompt,
        videoDuration: s.videoDuration || String(cfg.defaultDuration || ""),
      })))
      .catch(() => {});
    api.models()
      .then((res) => {
        setModels(res.models || []);
        setSettings((s) => ({ ...s, videoModel: s.videoModel || res.defaultModel }));
      })
      .catch((e) => setModelsError(e instanceof Error ? e.message : "Не удалось загрузить модели"));
    api.listAudio()
      .then((res) => {
        setAudio(res.assets || []);
        setSettings((s) =>
          s.audioAssetId && !res.assets.some((asset) => asset.id === s.audioAssetId)
            ? { ...s, audioAssetId: "" }
            : s,
        );
      })
      .catch((e) => setAudioError(e instanceof Error ? e.message : "Не удалось загрузить медиатеку"));
  }, []);

  const names = useMemo(() => normalizeCampaignList(namesText), [namesText]);
  const surnames = useMemo(() => normalizeCampaignList(surnamesText), [surnamesText]);
  const displayedNames = useMemo(
    () => nameSource === "Дефолтный список" && defaultNameDiagnostics
      ? { ...names, diagnostics: { ...names.diagnostics, duplicateCount: defaultNameDiagnostics.duplicateCount } }
      : names,
    [defaultNameDiagnostics, nameSource, names],
  );
  const displayedSurnames = useMemo(
    () => surnameSource === "Дефолтный список" && defaultSurnameDiagnostics
      ? { ...surnames, diagnostics: { ...surnames.diagnostics, duplicateCount: defaultSurnameDiagnostics.duplicateCount } }
      : surnames,
    [defaultSurnameDiagnostics, surnameSource, surnames],
  );
  const selectedModel = models.find((model) => model.id === settings.videoModel);

  function update<K extends keyof CampaignFormSettings>(key: K, value: CampaignFormSettings[K]) {
    setSettings((current) => ({ ...current, [key]: value }));
  }

  async function loadFile(file: File, tab: Tab) {
    if (!file.name.toLocaleLowerCase().endsWith(".txt")) {
      setError("Выберите текстовый файл .txt");
      return;
    }
    const text = await file.text();
    if (tab === "names") {
      setNamesText(text);
      setNameSource(`Файл: ${file.name}`);
    } else {
      setSurnamesText(text);
      setSurnameSource(`Файл: ${file.name}`);
    }
  }

  function onDrop(event: DragEvent<HTMLLabelElement>, tab: Tab) {
    event.preventDefault();
    const file = event.dataTransfer.files[0];
    if (file) void loadFile(file, tab);
  }

  function restore(tab: Tab) {
    if (tab === "names") {
      setNamesText(defaultNames.join("\n"));
      setNameSource("Дефолтный список");
    } else {
      setSurnamesText(defaultSurnames.join("\n"));
      setSurnameSource("Дефолтный список");
    }
  }

  async function submit() {
    setError("");
    if (!title.trim()) return setError("Укажите название кампании");
    if (names.values.length === 0 && surnames.values.length === 0) {
      return setError("Добавьте хотя бы одно имя или одну фамилию");
    }
    if (
      (names.values.length > 0 && !hasNamePlaceholder(nameTextTemplate)) ||
      (surnames.values.length > 0 && !hasNamePlaceholder(surnameTextTemplate))
    ) {
      return setError("Текст для каждого непустого списка должен содержать {{name}}");
    }
    if (!settings.templateId.trim()) return setError("Укажите ID шаблона Иманатора");
    if (!settings.generateAudio && !settings.audioAssetId) {
      return setError("Выберите mp3 или включите генерацию аудио моделью");
    }
    let imageSettings: Record<string, string>;
    try {
      const parsed: unknown = settings.imageSettingsText.trim()
        ? JSON.parse(settings.imageSettingsText)
        : {};
      if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") throw new Error();
      if (Object.values(parsed).some((value) => typeof value !== "string")) throw new Error();
      imageSettings = parsed as Record<string, string>;
    } catch {
      return setError("Доп. настройки должны быть JSON-объектом со строковыми значениями");
    }

    setSubmitting(true);
    try {
      const result = await api.createAdCampaign({
        title: title.trim(),
        names: names.values,
        surnames: surnames.values,
        nameTextTemplate: nameTextTemplate.trim(),
        surnameTextTemplate: surnameTextTemplate.trim(),
        templateId: settings.templateId.trim(),
        imageSettings,
        nameSettingKey: settings.nameSettingKey.trim() || "name",
        videoModel: settings.videoModel,
        videoPrompt: settings.videoPrompt,
        videoDuration: settings.videoDuration.trim()
          ? Number.parseInt(settings.videoDuration, 10)
          : null,
        videoResolution: settings.videoResolution,
        videoAspectRatio: settings.videoAspectRatio,
        generateAudio: settings.generateAudio,
        audioAssetId: settings.generateAudio ? "" : settings.audioAssetId,
      });
      navigate(`/campaigns/${result.campaign.id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось создать кампанию");
    } finally {
      setSubmitting(false);
    }
  }

  if (loadingDefaults) return <p className="text-sm text-slate-500">Загрузка списков…</p>;

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div>
        <Link to="/campaigns" className="text-sm text-slate-500 hover:text-blue-600">
          ← К кампаниям
        </Link>
        <h1 className="mt-2 text-xl font-bold text-slate-900">Новая рекламная кампания</h1>
        <p className="mt-1 text-sm text-slate-500">
          Кампания сохранится как черновик. Запуск выполняется отдельно после проверки.
        </p>
      </div>

      {defaultsError && <Alert>{defaultsError}</Alert>}

      <Field label="Название кампании">
        <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} required />
      </Field>

      <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
        <div className="flex overflow-x-auto border-b border-slate-200" role="tablist" aria-label="Тип списка">
          <TabButton active={activeTab === "names"} onClick={() => setActiveTab("names")}>
            Имена ({names.values.length})
          </TabButton>
          <TabButton active={activeTab === "surnames"} onClick={() => setActiveTab("surnames")}>
            Фамилии ({surnames.values.length})
          </TabButton>
        </div>
        <div className="space-y-4 p-4">
          {activeTab === "names" ? (
            <ListEditor
              label="Рекламный текст для имён"
              template={nameTextTemplate}
              onTemplateChange={setNameTextTemplate}
              text={namesText}
              onTextChange={(value) => {
                setNamesText(value);
                setNameSource("Введено вручную");
              }}
              normalized={displayedNames}
              source={nameSource}
              example={names.values[0] || "Анна"}
              onFile={(file) => void loadFile(file, "names")}
              onDrop={(event) => onDrop(event, "names")}
              onRestore={() => restore("names")}
            />
          ) : (
            <ListEditor
              label="Рекламный текст для фамилий"
              template={surnameTextTemplate}
              onTemplateChange={setSurnameTextTemplate}
              text={surnamesText}
              onTextChange={(value) => {
                setSurnamesText(value);
                setSurnameSource("Введено вручную");
              }}
              normalized={displayedSurnames}
              source={surnameSource}
              example={surnames.values[0] || "Иванова"}
              onFile={(file) => void loadFile(file, "surnames")}
              onDrop={(event) => onDrop(event, "surnames")}
              onRestore={() => restore("surnames")}
            />
          )}
        </div>
      </section>

      <section className="space-y-4 rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
        <h2 className="font-semibold text-slate-900">Общие настройки</h2>
        <Field label="ID шаблона Иманатора">
          <input className={inputCls} value={settings.templateId} onChange={(e) => update("templateId", e.target.value)} />
        </Field>
        <Field label="Ключ подстановки имени">
          <input className={inputCls} value={settings.nameSettingKey} onChange={(e) => update("nameSettingKey", e.target.value)} />
        </Field>
        <Field label="Доп. настройки изображения (JSON)">
          <textarea className={`${inputCls} min-h-20 font-mono text-sm`} value={settings.imageSettingsText} onChange={(e) => update("imageSettingsText", e.target.value)} />
        </Field>
        <hr className="border-slate-200" />
        <Field label="Модель видео (OpenRouter)">
          {modelsError ? (
            <input className={inputCls} value={settings.videoModel} onChange={(e) => update("videoModel", e.target.value)} />
          ) : (
            <select className={inputCls} value={settings.videoModel} onChange={(e) => update("videoModel", e.target.value)}>
              {models.length === 0 && <option value={settings.videoModel}>{settings.videoModel || "Загрузка…"}</option>}
              {models.map((model) => <option key={model.id} value={model.id}>{model.name} ({model.id})</option>)}
            </select>
          )}
          {modelsError && <p className="mt-1 text-xs text-amber-700">{modelsError}. Введите модель вручную.</p>}
        </Field>
        <Field label="Промпт для видео">
          <textarea className={`${inputCls} min-h-28`} value={settings.videoPrompt} onChange={(e) => update("videoPrompt", e.target.value)} />
        </Field>
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label="Длительность (сек)">
            <SelectOrInput value={settings.videoDuration} options={selectedModel?.supported_durations?.map(String)} onChange={(value) => update("videoDuration", value)} />
          </Field>
          <Field label="Разрешение">
            <SelectOrInput value={settings.videoResolution} options={selectedModel?.supported_resolutions} onChange={(value) => update("videoResolution", value)} />
          </Field>
          <Field label="Соотношение">
            <SelectOrInput value={settings.videoAspectRatio} options={selectedModel?.supported_aspect_ratios} onChange={(value) => update("videoAspectRatio", value)} />
          </Field>
        </div>
        <hr className="border-slate-200" />
        <Field label="Музыка (mp3 из медиатеки)">
          <select className={inputCls} value={settings.audioAssetId} onChange={(e) => update("audioAssetId", e.target.value)} disabled={settings.generateAudio}>
            <option value="">— не выбрано —</option>
            {audio.map((asset) => (
              <option key={asset.id} value={asset.id}>
                {asset.title || asset.filename} ({formatDuration(asset.durationSeconds)})
              </option>
            ))}
          </select>
          {audioError && <p className="mt-1 text-xs text-amber-700">{audioError}</p>}
          {!audioError && audio.length === 0 && (
            <p className="mt-1 text-xs text-amber-700">
              Медиатека пуста — <Link to="/media" className="text-blue-600 hover:underline">загрузите mp3</Link>.
            </p>
          )}
        </Field>
        <label className="flex cursor-pointer items-start gap-2.5">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4"
            checked={settings.generateAudio}
            onChange={(e) => e.target.checked ? setAudioWarnOpen(true) : update("generateAudio", false)}
          />
          <span className="text-sm text-slate-700">
            Генерировать аудио моделью
            <span className="block text-xs text-slate-500">Существенно дороже, музыка из медиатеки не накладывается.</span>
          </span>
        </label>
      </section>

      {error && <Alert>{error}</Alert>}
      <button
        type="button"
        onClick={() => void submit()}
        disabled={submitting || Boolean(defaultsError)}
        className="w-full rounded-xl bg-blue-600 py-2.5 font-semibold text-white shadow-sm transition hover:bg-blue-500 disabled:opacity-50"
      >
        {submitting ? "Сохраняем…" : `Создать черновик (${names.values.length + surnames.values.length} задач)`}
      </button>

      <ConfirmDialog
        open={audioWarnOpen}
        title="Включить генерацию аудио моделью?"
        confirmLabel="Включить"
        onCancel={() => setAudioWarnOpen(false)}
        onConfirm={() => {
          update("generateAudio", true);
          setAudioWarnOpen(false);
        }}
      >
        <p>Видео со звуком от модели стоит существенно дороже на каждой задаче кампании.</p>
      </ConfirmDialog>
    </div>
  );
}

function ListEditor({
  label,
  template,
  onTemplateChange,
  text,
  onTextChange,
  normalized,
  source,
  example,
  onFile,
  onDrop,
  onRestore,
}: {
  label: string;
  template: string;
  onTemplateChange: (value: string) => void;
  text: string;
  onTextChange: (value: string) => void;
  normalized: NormalizedList;
  source: string;
  example: string;
  onFile: (file: File) => void;
  onDrop: (event: DragEvent<HTMLLabelElement>) => void;
  onRestore: () => void;
}) {
  return (
    <>
      <Field label={`${label} (используйте {{name}})`}>
        <textarea className={`${inputCls} min-h-24`} value={template} onChange={(e) => onTemplateChange(e.target.value)} />
        <p className="mt-1 text-xs text-slate-500">
          Пример: {applyNamePreview(template, example)}
        </p>
        {normalized.values.length > 0 && !hasNamePlaceholder(template) && (
          <p className="mt-1 text-xs text-red-600">Добавьте {"{{name}}"} в текст.</p>
        )}
      </Field>
      <label
        className="block cursor-pointer rounded-xl border-2 border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-center transition hover:border-blue-400 hover:bg-blue-50"
        onDragOver={(event) => event.preventDefault()}
        onDrop={onDrop}
      >
        <span className="block text-sm font-medium text-slate-700">Перетащите .txt или выберите файл</span>
        <span className="mt-1 block text-xs text-slate-500">Одна строка — один элемент</span>
        <input type="file" accept=".txt,text/plain" className="sr-only" onChange={(e) => e.target.files?.[0] && onFile(e.target.files[0])} />
      </label>
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
        <span className="rounded-full bg-slate-100 px-2.5 py-1 text-slate-600">{source}</span>
        <button type="button" onClick={onRestore} className="font-medium text-blue-600 hover:underline">
          Восстановить дефолтный список
        </button>
      </div>
      <Field label={`Список построчно (${normalized.values.length})`}>
        <textarea className={`${inputCls} min-h-48 font-mono text-sm`} value={text} onChange={(e) => onTextChange(e.target.value)} />
      </Field>
      <Diagnostics normalized={normalized} />
    </>
  );
}

function Diagnostics({ normalized }: { normalized: NormalizedList }) {
  const { duplicateCount, invalidCapitalization } = normalized.diagnostics;
  if (!duplicateCount && invalidCapitalization.length === 0) return null;
  return (
    <div className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
      {duplicateCount > 0 && <p>Удалено дублей: {duplicateCount}</p>}
      {invalidCapitalization.length > 0 && (
        <p>
          Со строчной буквы: {invalidCapitalization.length}. Примеры:{" "}
          {invalidCapitalization.slice(0, 5).join(", ")}
        </p>
      )}
    </div>
  );
}

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`shrink-0 border-b-2 px-5 py-3 text-sm font-medium transition ${
        active ? "border-blue-600 text-blue-700" : "border-transparent text-slate-500 hover:text-slate-800"
      }`}
    >
      {children}
    </button>
  );
}

function Alert({ children }: { children: React.ReactNode }) {
  return <div className="rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">{children}</div>;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm text-slate-500">{label}</span>
      {children}
    </label>
  );
}

function SelectOrInput({ value, options, onChange }: { value: string; options?: string[]; onChange: (value: string) => void }) {
  if (options?.length) {
    return (
      <select className={inputCls} value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">авто</option>
        {options.map((option) => <option key={option} value={option}>{option}</option>)}
      </select>
    );
  }
  return <input className={inputCls} value={value} onChange={(e) => onChange(e.target.value)} placeholder="авто" />;
}

const inputCls =
  "w-full rounded-xl border border-slate-300 bg-white px-3 py-2 outline-none transition focus:border-blue-500 focus:ring-2 focus:ring-blue-100";
