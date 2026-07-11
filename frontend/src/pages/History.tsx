import { useEffect, useRef, useState } from "react";
import { api, type Batch, type Task } from "../api";
import { isActive } from "../status";
import { formatRub, formatUsd } from "../format";
import TaskTable from "../components/TaskTable";

export default function History() {
  const [batches, setBatches] = useState<Batch[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [openId, setOpenId] = useState<string | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const pollRef = useRef<number | null>(null);

  async function loadBatches() {
    try {
      const res = await api.listBatches();
      setBatches(res.batches);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Ошибка загрузки");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadBatches();
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current);
    };
  }, []);

  function stopPolling() {
    if (pollRef.current) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }

  function toggle(batchId: string) {
    stopPolling();
    if (openId === batchId) {
      setOpenId(null);
      setTasks([]);
      return;
    }
    setOpenId(batchId);
    setTasks([]);
    const tick = async () => {
      const res = await api.listTasks(batchId);
      setTasks(res.tasks);
      if (res.tasks.length > 0 && res.tasks.every((t) => !isActive(t.status))) {
        stopPolling();
        loadBatches();
      }
    };
    tick().catch(() => {});
    pollRef.current = window.setInterval(() => tick().catch(() => {}), 3000);
  }

  async function remove(batchId: string) {
    if (!window.confirm("Удалить эту пачку и все её видео безвозвратно?")) return;
    setDeletingId(batchId);
    try {
      await api.deleteBatch(batchId);
      if (openId === batchId) {
        stopPolling();
        setOpenId(null);
        setTasks([]);
      }
      setBatches((prev) => prev.filter((b) => b.id !== batchId));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось удалить пачку");
    } finally {
      setDeletingId(null);
    }
  }

  if (loading) return <p className="text-slate-500">Загрузка…</p>;

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-bold text-slate-900">История пачек 🗂️</h1>
      {error && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-xl px-3 py-2">
          {error}
        </div>
      )}
      {batches.length === 0 && <p className="text-slate-500 text-sm">Пачек пока нет.</p>}

      <div className="space-y-3">
        {batches.map((b) => (
          <div
            key={b.id}
            className="rounded-2xl border border-slate-200 bg-white shadow-sm overflow-hidden"
          >
            <div className="flex items-center gap-2 pr-2">
              <button
                onClick={() => toggle(b.id)}
                className="flex-1 flex items-center gap-4 px-4 py-3 hover:bg-blue-50/40 transition text-left min-w-0"
              >
                <span className="text-blue-500">{openId === b.id ? "▾" : "▸"}</span>
                <div className="flex-1 min-w-0">
                  <div className="font-semibold text-slate-800 truncate">
                    {b.title || "Без названия"}
                  </div>
                  <div className="text-xs text-slate-500">
                    {new Date(b.createdAt).toLocaleString("ru-RU")} · модель {b.videoModel}
                  </div>
                </div>
                <div className="flex items-center gap-3 text-xs whitespace-nowrap">
                  {b.costUsd > 0 && (
                    <span
                      className="text-slate-600"
                      title={`${formatUsd(b.costUsd)} по текущему курсу`}
                    >
                      {formatRub(b.costRub)}
                    </span>
                  )}
                  <span className="text-emerald-600">{b.done} готово</span>
                  {b.failed > 0 && <span className="text-red-500">{b.failed} ошибок</span>}
                  <span className="text-slate-400">из {b.total}</span>
                </div>
              </button>
              <button
                onClick={() => remove(b.id)}
                disabled={deletingId === b.id}
                title="Удалить пачку"
                className="shrink-0 px-2.5 py-1.5 rounded-lg text-sm text-slate-400 hover:text-red-600 hover:bg-red-50 disabled:opacity-50 transition"
              >
                {deletingId === b.id ? "…" : "🗑"}
              </button>
            </div>
            {openId === b.id && (
              <div className="px-4 pb-4">
                <TaskTable tasks={tasks} />
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
