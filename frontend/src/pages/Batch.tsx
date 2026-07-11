import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type Batch, type Task } from "../api";
import { isActive } from "../status";
import { formatRub, formatUsd } from "../format";
import TaskTable from "../components/TaskTable";

export default function BatchPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();

  const [batch, setBatch] = useState<Batch | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deleting, setDeleting] = useState(false);
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
  }, [id]);

  const first = tasks[0];
  const namesText = useMemo(
    () => tasks.map((t) => `${t.firstName} ${t.lastName}`.trim()).join("\n"),
    [tasks],
  );
  const extraSettings = useMemo(() => {
    if (!first?.imageSettings) return "";
    return JSON.stringify(first.imageSettings, null, 2);
  }, [first]);

  const doneCount = tasks.filter((t) => t.status === "done").length;
  const totalCostUsd = tasks.reduce((sum, t) => sum + (t.costUsd || 0), 0);
  const totalCostRub = tasks.reduce((sum, t) => sum + (t.costRub || 0), 0);

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

  if (loading && tasks.length === 0 && !batch) {
    return <p className="text-slate-500">Загрузка…</p>;
  }

  return (
    <div className="space-y-6">
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

      <div className="rounded-xl bg-blue-50 border border-blue-200 text-blue-700 text-sm px-4 py-2.5">
        🔒 Пачка уже отправлена на генерацию — список ФИО и настройки изменить нельзя.
      </div>

      <div className="grid lg:grid-cols-[1fr_1.1fr] gap-6">
        {/* left: read-only settings */}
        <section className="space-y-5">
          <h2 className="text-lg font-semibold text-slate-900">Параметры пачки</h2>

          <Field label={`Список ФИО (${tasks.length})`}>
            <textarea
              className={`${roCls} min-h-40 font-mono text-sm`}
              value={namesText}
              readOnly
            />
          </Field>

          <Field label="ID шаблона Иманатора">
            <input className={roCls} value={first?.templateId || batch?.templateId || ""} readOnly />
          </Field>

          <Field label="Модель видео (OpenRouter)">
            <input className={roCls} value={first?.videoModel || batch?.videoModel || ""} readOnly />
          </Field>

          <Field label="Промпт для видео">
            <textarea className={`${roCls} min-h-28 text-sm`} value={first?.videoPrompt || ""} readOnly />
          </Field>

          <div className="grid grid-cols-3 gap-3">
            <Field label="Длительность (сек)">
              <input className={roCls} value={first?.videoDuration ?? "авто"} readOnly />
            </Field>
            <Field label="Разрешение">
              <input className={roCls} value={first?.videoResolution || "авто"} readOnly />
            </Field>
            <Field label="Соотношение">
              <input className={roCls} value={first?.videoAspectRatio || "авто"} readOnly />
            </Field>
          </div>

          {extraSettings && (
            <Field label="Настройки шаблона (снимок)">
              <textarea className={`${roCls} font-mono text-xs min-h-24`} value={extraSettings} readOnly />
            </Field>
          )}
        </section>

        {/* right: results */}
        <section className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h2 className="text-lg font-semibold text-slate-900">Результаты</h2>
            <div className="flex items-center gap-3 text-xs">
              {totalCostUsd > 0 && (
                <span className="text-slate-600" title={formatUsd(totalCostUsd)}>
                  {formatRub(totalCostRub)}
                </span>
              )}
              <span className="text-slate-500">
                {doneCount}/{tasks.length} готово
              </span>
            </div>
          </div>
          <TaskTable tasks={tasks} />
        </section>
      </div>
    </div>
  );
}

const roCls =
  "w-full px-3 py-2 rounded-xl bg-slate-100 border border-slate-200 text-slate-600 outline-none cursor-not-allowed";

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="block text-sm text-slate-500 mb-1">{label}</span>
      {children}
    </label>
  );
}
