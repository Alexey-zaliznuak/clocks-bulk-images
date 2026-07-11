import { useEffect, useMemo, useRef, useState } from "react";
import { api, type Task, type VideoModel } from "../api";
import { loadSettings, parseNames, saveSettings, type UiSettings } from "../settings";
import { isActive } from "../status";
import TaskTable from "../components/TaskTable";

export default function Create() {
  const [settings, setSettings] = useState<UiSettings>(loadSettings);
  const [models, setModels] = useState<VideoModel[]>([]);
  const [modelsError, setModelsError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const [currentBatchId, setCurrentBatchId] = useState<string | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const pollRef = useRef<number | null>(null);

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
      }));
    }).catch(() => {});

    api.models()
      .then((res) => {
        setModels(res.models);
        setSettings((s) => ({ ...s, videoModel: s.videoModel || res.defaultModel }));
      })
      .catch((e) => setModelsError(e.message || "Не удалось загрузить модели"));
  }, []);

  const parsed = useMemo(() => parseNames(settings.namesText), [settings.namesText]);

  function update<K extends keyof UiSettings>(key: K, value: UiSettings[K]) {
    setSettings((s) => ({ ...s, [key]: value }));
  }

  function startPolling(batchId: string) {
    if (pollRef.current) window.clearInterval(pollRef.current);
    const tick = async () => {
      try {
        const res = await api.listTasks(batchId);
        setTasks(res.tasks);
        if (res.tasks.length > 0 && res.tasks.every((t) => !isActive(t.status))) {
          if (pollRef.current) window.clearInterval(pollRef.current);
          pollRef.current = null;
        }
      } catch {
        /* keep polling */
      }
    };
    tick();
    pollRef.current = window.setInterval(tick, 2000);
  }

  useEffect(() => {
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current);
    };
  }, []);

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
        extraSettings,
        firstNameKey: settings.firstNameKey,
        lastNameKey: settings.lastNameKey,
        fullNameKey: settings.fullNameKey,
        names: parsed,
      });
      setCurrentBatchId(res.batchId);
      setTasks([]);
      startPolling(res.batchId);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Ошибка создания задач");
    } finally {
      setSubmitting(false);
    }
  }

  const selectedModel = models.find((m) => m.id === settings.videoModel);

  return (
    <div className="grid lg:grid-cols-[1fr_1.1fr] gap-6">
      {/* left: form */}
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
              placeholder="google/veo-3.1"
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

      {/* right: live results */}
      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-bold text-slate-900">Прогресс</h2>
          {currentBatchId && (
            <span className="text-xs text-slate-500">
              {tasks.filter((t) => t.status === "done").length}/{tasks.length} готово
            </span>
          )}
        </div>
        {currentBatchId ? (
          <TaskTable tasks={tasks} />
        ) : (
          <div className="rounded-2xl border border-dashed border-slate-300 bg-white p-8 text-center text-slate-500 text-sm">
            Здесь появятся задачи и ссылки на результаты по мере готовности.
          </div>
        )}
      </section>
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
