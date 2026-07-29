import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, type MediaAsset, type VideoModel } from "../api";
import { loadSettings, parseNames, saveSettings, type UiSettings } from "../settings";
import ConfirmDialog from "../components/ConfirmDialog";
import { formatDuration } from "../format";

export default function Create() {
  const navigate = useNavigate();
  const [settings, setSettings] = useState<UiSettings>(loadSettings);
  const [models, setModels] = useState<VideoModel[]>([]);
  const [modelsError, setModelsError] = useState("");
  const [audio, setAudio] = useState<MediaAsset[]>([]);
  const [audioError, setAudioError] = useState("");
  const [audioWarnOpen, setAudioWarnOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  // Persist every settings change to localStorage.
  useEffect(() => {
    saveSettings(settings);
  }, [settings]);

  // Load defaults (prompt/model) and the OpenRouter model list.
  useEffect(() => {
    api.config().then((cfg) => {
      setSettings((s) => ({
        ...s,
        videoPrompt: s.videoPrompt || cfg.defaultPrompt,
        videoModel: s.videoModel || cfg.defaultModel,
        videoDuration: s.videoDuration || String(cfg.defaultDuration || ""),
      }));
    }).catch(() => {});

    api.models()
      .then((res) => {
        setModels(res.models);
        setSettings((s) => ({ ...s, videoModel: s.videoModel || res.defaultModel }));
      })
      .catch((e) => setModelsError(e.message || "Не удалось загрузить модели"));

    api.listAudio()
      .then((res) => {
        const assets = res.assets || [];
        setAudio(assets);
        // Drop a selection pointing at a file that no longer exists.
        setSettings((s) =>
          s.audioAssetId && !assets.some((a) => a.id === s.audioAssetId)
            ? { ...s, audioAssetId: "" }
            : s
        );
      })
      .catch((e) => setAudioError(e.message || "Не удалось загрузить медиатеку"));
  }, []);

  const parsed = useMemo(() => parseNames(settings.namesText), [settings.namesText]);

  function update<K extends keyof UiSettings>(key: K, value: UiSettings[K]) {
    setSettings((s) => ({ ...s, [key]: value }));
  }

  // Turning model audio on is expensive, so it always goes through a warning.
  function toggleGenerateAudio(next: boolean) {
    if (next) {
      setAudioWarnOpen(true);
      return;
    }
    update("generateAudio", false);
  }

  async function onSubmit() {
    setError("");
    if (!settings.templateId.trim()) {
      setError("Укажите ID шаблона Иманатора");
      return;
    }
    if (parsed.length === 0) {
      setError("Список ФИО пуст");
      return;
    }
    if (!settings.generateAudio && !settings.audioAssetId) {
      setError("Выберите mp3 для озвучки — видео генерируется без звука");
      return;
    }
    let extraSettings: Record<string, string> = {};
    try {
      extraSettings = settings.extraSettingsText.trim()
        ? JSON.parse(settings.extraSettingsText)
        : {};
    } catch {
      setError("Доп. настройки должны быть валидным JSON-объектом");
      return;
    }

    setSubmitting(true);
    try {
      const duration = settings.videoDuration.trim()
        ? parseInt(settings.videoDuration, 10)
        : null;
      const res = await api.createBatch({
        title: settings.title,
        templateId: settings.templateId.trim(),
        videoModel: settings.videoModel,
        videoPrompt: settings.videoPrompt,
        videoDuration: duration,
        videoResolution: settings.videoResolution,
        videoAspectRatio: settings.videoAspectRatio,
        generateAudio: settings.generateAudio,
        audioAssetId: settings.generateAudio ? "" : settings.audioAssetId,
        extraSettings,
        firstNameKey: settings.firstNameKey,
        lastNameKey: settings.lastNameKey,
        fullNameKey: settings.fullNameKey,
        names: parsed,
      });
      navigate(`/batch/${res.batchId}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Ошибка создания задач");
    } finally {
      setSubmitting(false);
    }
  }

  const selectedModel = models.find((m) => m.id === settings.videoModel);
  const selectedAudio = audio.find((a) => a.id === settings.audioAssetId);

  // Warn up front when the chosen track would stretch the clip a lot: the
  // backend refuses anything past its limit.
  const stretchNote = useMemo(() => {
    const clip = parseInt(settings.videoDuration, 10);
    if (!selectedAudio || !clip || clip <= 0) return "";
    const factor = selectedAudio.durationSeconds / clip;
    if (factor > 6) {
      return `замедление в ${factor.toFixed(1)} раза, сервер такое отклонит (лимит 6x)`;
    }
    if (factor > 2.5) {
      return `замедление в ${factor.toFixed(1)} раза — кадры досинтезируются, на быстром движении возможны артефакты`;
    }
    if (factor < 1) {
      return `ускорение в ${(1 / factor).toFixed(1)} раза`;
    }
    return `замедление в ${factor.toFixed(1)} раза`;
  }, [selectedAudio, settings.videoDuration]);

  return (
    <div className="max-w-2xl mx-auto">
      <section className="space-y-5">
        <div>
          <h1 className="text-xl font-bold text-slate-900 mb-1">Новая пачка задач ✨</h1>
          <p className="text-sm text-slate-500">
            Каждая строка — одно ФИО. Для каждого будет: изображение в Иманаторе → видео в OpenRouter → сохранение в хранилище.
          </p>
        </div>

        <Field label="Название пачки (необязательно)">
          <input
            className={inputCls}
            value={settings.title}
            onChange={(e) => update("title", e.target.value)}
            placeholder="Партия от 11.07"
          />
        </Field>

        <Field label={`Список ФИО построчно (${parsed.length})`}>
          <textarea
            className={`${inputCls} min-h-40 font-mono text-sm`}
            value={settings.namesText}
            onChange={(e) => update("namesText", e.target.value)}
            placeholder={"Иван Иванов\nМария Петрова\n..."}
          />
        </Field>

        <Field label="ID шаблона Иманатора">
          <input
            className={inputCls}
            value={settings.templateId}
            onChange={(e) => update("templateId", e.target.value)}
            placeholder="uuid шаблона"
          />
        </Field>

        <div className="grid grid-cols-3 gap-3">
          <Field label="Ключ имени">
            <input className={inputCls} value={settings.firstNameKey} onChange={(e) => update("firstNameKey", e.target.value)} />
          </Field>
          <Field label="Ключ фамилии">
            <input className={inputCls} value={settings.lastNameKey} onChange={(e) => update("lastNameKey", e.target.value)} />
          </Field>
          <Field label="Ключ ФИО">
            <input className={inputCls} value={settings.fullNameKey} onChange={(e) => update("fullNameKey", e.target.value)} />
          </Field>
        </div>

        <Field label="Доп. настройки шаблона (JSON)">
          <textarea
            className={`${inputCls} font-mono text-sm min-h-20`}
            value={settings.extraSettingsText}
            onChange={(e) => update("extraSettingsText", e.target.value)}
            placeholder='{"city": "Москва"}'
          />
        </Field>

        <hr className="border-slate-200" />

        <Field label="Модель видео (OpenRouter)">
          {modelsError ? (
            <input
              className={inputCls}
              value={settings.videoModel}
              onChange={(e) => update("videoModel", e.target.value)}
              placeholder="google/veo-3.1-lite"
            />
          ) : (
            <select
              className={inputCls}
              value={settings.videoModel}
              onChange={(e) => update("videoModel", e.target.value)}
            >
              {models.length === 0 && <option value={settings.videoModel}>{settings.videoModel || "Загрузка…"}</option>}
              {models.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name} ({m.id})
                </option>
              ))}
            </select>
          )}
          {modelsError && <p className="text-xs text-amber-600 mt-1">{modelsError}. Введите модель вручную.</p>}
        </Field>

        <Field label="Промпт для видео">
          <textarea
            className={`${inputCls} min-h-28 text-sm`}
            value={settings.videoPrompt}
            onChange={(e) => update("videoPrompt", e.target.value)}
          />
        </Field>

        <div className="grid grid-cols-3 gap-3">
          <Field label="Длительность (сек)">
            <SelectOrInput
              value={settings.videoDuration}
              options={selectedModel?.supported_durations?.map(String)}
              onChange={(v) => update("videoDuration", v)}
              placeholder="авто"
            />
          </Field>
          <Field label="Разрешение">
            <SelectOrInput
              value={settings.videoResolution}
              options={selectedModel?.supported_resolutions}
              onChange={(v) => update("videoResolution", v)}
              placeholder="авто"
            />
          </Field>
          <Field label="Соотношение">
            <SelectOrInput
              value={settings.videoAspectRatio}
              options={selectedModel?.supported_aspect_ratios}
              onChange={(v) => update("videoAspectRatio", v)}
              placeholder="авто"
            />
          </Field>
        </div>

        <hr className="border-slate-200" />

        <div className="space-y-3">
          <div>
            <h2 className="text-sm font-semibold text-slate-800">Звук</h2>
            <p className="text-xs text-slate-500 mt-0.5">
              Модель генерирует видео без звука — музыка накладывается после, а видео
              подгоняется точно под длительность выбранного mp3. Чем ближе длина трека к
              длине клипа, тем меньше кадров приходится досинтезировать.
            </p>
          </div>

          <Field label="Музыка (mp3 из медиатеки)">
            <select
              className={inputCls}
              value={settings.audioAssetId}
              onChange={(e) => update("audioAssetId", e.target.value)}
              disabled={settings.generateAudio}
            >
              <option value="">— не выбрано —</option>
              {audio.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.title || a.filename} ({formatDuration(a.durationSeconds)})
                </option>
              ))}
            </select>
            {audioError && <p className="text-xs text-amber-600 mt-1">{audioError}</p>}
            {!audioError && audio.length === 0 && (
              <p className="text-xs text-amber-600 mt-1">
                Медиатека пуста —{" "}
                <Link to="/media" className="text-blue-600 hover:underline">
                  загрузите mp3
                </Link>
                .
              </p>
            )}
            {selectedAudio && (
              <p className="text-xs text-slate-500 mt-1">
                Итоговое видео будет длиться {formatDuration(selectedAudio.durationSeconds)}
                {stretchNote && ` — ${stretchNote}`}
              </p>
            )}
          </Field>

          <label className="flex items-start gap-2.5 cursor-pointer">
            <input
              type="checkbox"
              className="mt-0.5 h-4 w-4 rounded border-slate-300 text-amber-600 focus:ring-amber-200"
              checked={settings.generateAudio}
              onChange={(e) => toggleGenerateAudio(e.target.checked)}
            />
            <span className="text-sm text-slate-700">
              Генерировать аудио моделью
              <span className="block text-xs text-slate-500">
                Существенно дороже, музыка из медиатеки не накладывается.
              </span>
            </span>
          </label>
        </div>

        {error && (
          <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-xl px-3 py-2">
            {error}
          </div>
        )}

        <button
          onClick={onSubmit}
          disabled={submitting}
          className="w-full py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-semibold shadow-sm transition"
        >
          {submitting ? "Создаём…" : `Запустить ${parsed.length} задач`}
        </button>
      </section>

      <ConfirmDialog
        open={audioWarnOpen}
        title="Включить генерацию аудио моделью?"
        confirmLabel="Всё равно включить"
        onCancel={() => setAudioWarnOpen(false)}
        onConfirm={() => {
          update("generateAudio", true);
          setAudioWarnOpen(false);
        }}
      >
        <p>
          Видео со звуком от модели стоит существенно дороже, чем без него — цена
          вырастет на каждой задаче пачки.
        </p>
        <p>
          Обычный путь дешевле: модель отдаёт короткий клип без звука, а музыка из
          медиатеки накладывается уже на нашей стороне. Включайте это только если
          нужен именно сгенерированный звук.
        </p>
      </ConfirmDialog>
    </div>
  );
}

const inputCls =
  "w-full px-3 py-2 rounded-xl bg-white border border-slate-300 focus:border-blue-500 focus:ring-2 focus:ring-blue-100 outline-none transition";

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="block text-sm text-slate-500 mb-1">{label}</span>
      {children}
    </label>
  );
}

function SelectOrInput({
  value,
  options,
  onChange,
  placeholder,
}: {
  value: string;
  options?: string[];
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  if (options && options.length > 0) {
    return (
      <select className={inputCls} value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">{placeholder || "авто"}</option>
        {options.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
    );
  }
  return (
    <input
      className={inputCls}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
    />
  );
}
