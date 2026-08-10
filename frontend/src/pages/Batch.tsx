import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type Batch, type MediaAsset, type Task } from "../api";
import { isActive } from "../status";
import { formatDuration, formatRub, formatUsd } from "../format";
import TaskTable from "../components/TaskTable";

export default function BatchPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();

  const [batch, setBatch] = useState<Batch | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [audioAssets, setAudioAssets] = useState<MediaAsset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [retryMessage, setRetryMessage] = useState("");
  // Bumped after a retry so the polling effect restarts.
  const [refreshKey, setRefreshKey] = useState(0);
  const pollRef = useRef<number | null>(null);

  function stopPolling() {
    if (pollRef.current) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }

  async function loadBatch() {
    try {
      const res = await api.getBatch(id);
      setBatch(res.batch);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось загрузить пачку");
    }
  }

  // Loaded once to show the soundtrack by name instead of its storage key.
  useEffect(() => {
    api.listAudio()
      .then((res) => setAudioAssets(res.assets || []))
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    loadBatch();

    const tick = async () => {
      try {
        const res = await api.listTasks(id);
        setTasks(res.tasks);
        if (res.tasks.length > 0 && res.tasks.every((t) => !isActive(t.status))) {
          stopPolling();
          loadBatch();
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : "Ошибка загрузки задач");
      } finally {
        setLoading(false);
      }
    };
    tick();
    pollRef.current = window.setInterval(tick, 3000);
    return () => stopPolling();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, refreshKey]);

  const first = tasks[0];
  const namesText = useMemo(
    () => tasks.map((t) => `${t.firstName} ${t.lastName}`.trim()).join("\n"),
    [tasks],
  );
  const extraSettings = useMemo(() => {
    if (!first?.imageSettings) return "";
    return JSON.stringify(first.imageSettings, null, 2);
  }, [first]);

  const audioLabel = useMemo(() => {
    if (!first) return "";
    if (first.generateAudio) return "сгенерирован моделью";
    const asset = audioAssets.find((a) => a.id === first.audioAssetId);
    if (asset) {
      return `${asset.title || asset.filename} (${formatDuration(asset.durationSeconds)})`;
    }
    // The track may have been deleted from the library since the batch ran.
    return first.audioObject || "—";
  }, [first, audioAssets]);

  // Short one-line recap shown while the settings block stays collapsed.
  const summaryChips = useMemo(() => {
    const chips = [`${tasks.length} ФИО`];
    const model = first?.videoModel || batch?.videoModel;
    if (model) chips.push(model);
    if (first?.videoDuration) chips.push(`${first.videoDuration} сек`);
    if (first?.videoResolution) chips.push(first.videoResolution);
    if (first?.videoAspectRatio) chips.push(first.videoAspectRatio);
    return chips;
  }, [tasks.length, first, batch]);

  const doneCount = tasks.filter((t) => t.status === "done").length;
  const failedCount = tasks.filter((t) => t.status === "failed").length;
  const totalCostUsd = tasks.reduce((sum, t) => sum + (t.costUsd || 0), 0);
  const totalCostRub = tasks.reduce((sum, t) => sum + (t.costRub || 0), 0);

  async function retryFailed() {
    setError("");
    setRetryMessage("");
    setRetrying(true);
    try {
      const result = await api.retryBatch(id);
      setRetryMessage(`Перезапущено задач: ${result.retried}`);
      setRefreshKey((k) => k + 1);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось перезапустить задачи");
    } finally {
      setRetrying(false);
    }
  }

  async function remove() {
    if (!window.confirm("Удалить эту пачку и все её видео безвозвратно?")) return;
    setDeleting(true);
    try {
      await api.deleteBatch(id);
      stopPolling();
      navigate("/history");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось удалить пачку");
      setDeleting(false);
    }
  }

  async function downloadArchive() {
    setError("");
    setArchiving(true);
    try {
      const { downloadUrl } = await api.createBatchArchive(id);
      const a = document.createElement("a");
      a.href = downloadUrl;
      a.rel = "noopener";
      document.body.appendChild(a);
      a.click();
      a.remove();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось собрать архив");
    } finally {
      setArchiving(false);
    }
  }

  if (loading && tasks.length === 0 && !batch) {
    return <p className="text-slate-500">Загрузка…</p>;
  }

  return (
    <div className="space-y-4">
      {/* header */}
      <div className="flex flex-wrap items-center gap-3">
        <Link
          to="/history"
          className="text-sm text-slate-500 hover:text-blue-600 transition"
        >
          ← К истории
        </Link>
        <div className="flex-1 min-w-0">
          <h1 className="text-xl font-bold text-slate-900 truncate">
            {batch?.title || "Без названия"}
          </h1>
          {batch && (
            <p className="text-xs text-slate-500">
              {new Date(batch.createdAt).toLocaleString("ru-RU")} · модель {batch.videoModel}
            </p>
          )}
        </div>
        {failedCount > 0 && (
          <button
            onClick={retryFailed}
            disabled={retrying}
            className="px-4 py-2 rounded-xl text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 transition"
          >
            {retrying ? "Перезапускаем…" : `↻ Повторить все ошибки (${failedCount})`}
          </button>
        )}
        <button
          onClick={remove}
          disabled={deleting}
          className="px-3 py-1.5 rounded-full text-sm text-slate-500 hover:text-red-600 hover:bg-red-50 disabled:opacity-50 transition"
        >
          {deleting ? "Удаляем…" : "🗑 Удалить"}
        </button>
      </div>

      {error && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-xl px-3 py-2">
          {error}
        </div>
      )}
      {retryMessage && (
        <div className="text-sm text-emerald-700 bg-emerald-50 border border-emerald-200 rounded-xl px-3 py-2">
          {retryMessage}
        </div>
      )}

      {/* read-only settings, collapsed so results get the full width */}
      <details className="group rounded-2xl border border-slate-200 bg-white shadow-sm">
        <summary className="flex flex-wrap items-center gap-x-2 gap-y-1 px-4 py-2.5 cursor-pointer list-none [&::-webkit-details-marker]:hidden">
          <span className="text-blue-500 text-xs transition group-open:rotate-90">▸</span>
          <span className="text-sm font-semibold text-slate-900">Параметры пачки</span>
          <span className="min-w-0 truncate text-xs text-slate-500 group-open:hidden">
            {summaryChips.join(" · ")}
          </span>
          <span className="ml-auto text-xs text-slate-400 shrink-0">
            🔒 только просмотр
          </span>
        </summary>

        <div className="border-t border-slate-100 px-4 py-4 grid md:grid-cols-2 gap-x-4 gap-y-3">
          <Field label={`Список ФИО (${tasks.length})`}>
            <textarea
              className={`${roCls} min-h-24 font-mono text-xs`}
              value={namesText}
              readOnly
            />
          </Field>

          <Field label="Промпт для видео">
            <textarea className={`${roCls} min-h-24 text-xs`} value={first?.videoPrompt || ""} readOnly />
          </Field>

          <Field label="ID шаблона Иманатора">
            <input className={roCls} value={first?.templateId || batch?.templateId || ""} readOnly />
          </Field>

          <Field label="Модель видео (OpenRouter)">
            <input className={roCls} value={first?.videoModel || batch?.videoModel || ""} readOnly />
          </Field>

          <div className="grid grid-cols-3 gap-2">
            <Field label="Длительность">
              <input className={roCls} value={first?.videoDuration ?? "авто"} readOnly />
            </Field>
            <Field label="Разрешение">
              <input className={roCls} value={first?.videoResolution || "авто"} readOnly />
            </Field>
            <Field label="Соотношение">
              <input className={roCls} value={first?.videoAspectRatio || "авто"} readOnly />
            </Field>
          </div>

          <Field label="Звук">
            <input className={roCls} value={audioLabel} readOnly />
          </Field>

          {extraSettings && (
            <Field label="Настройки шаблона (снимок)">
              <textarea className={`${roCls} font-mono text-xs min-h-24`} value={extraSettings} readOnly />
            </Field>
          )}
        </div>
      </details>

      <section className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold text-slate-900">Результаты</h2>
          <div className="flex flex-wrap items-center gap-3 text-xs">
            {totalCostUsd > 0 && (
              <span className="text-slate-600" title={formatUsd(totalCostUsd)}>
                {formatRub(totalCostRub)}
              </span>
            )}
            <span className="text-slate-500">
              {doneCount}/{tasks.length} готово
            </span>
            {doneCount > 0 && (
              <button
                onClick={downloadArchive}
                disabled={archiving}
                className="px-3 py-1.5 rounded-xl text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 transition"
              >
                {archiving ? "Собираем архив…" : "↓ Скачать архив"}
              </button>
            )}
          </div>
        </div>
        <TaskTable tasks={tasks} onRetried={() => setRefreshKey((k) => k + 1)} />
      </section>
    </div>
  );
}

const roCls =
  "w-full px-3 py-1.5 rounded-xl bg-slate-100 border border-slate-200 text-slate-600 text-sm outline-none cursor-not-allowed";

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="block text-xs text-slate-500 mb-1">{label}</span>
      {children}
    </label>
  );
}
