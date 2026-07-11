import { useEffect, useRef, useState } from "react";
import { api, type Batch, type Task } from "../api";
import { isActive } from "../status";
import TaskTable from "../components/TaskTable";

export default function History() {
  const [batches, setBatches] = useState<Batch[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [openId, setOpenId] = useState<string | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
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

  function toggle(batchId: string) {
    if (pollRef.current) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
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
        if (pollRef.current) window.clearInterval(pollRef.current);
        pollRef.current = null;
        loadBatches();
      }
    };
    tick().catch(() => {});
    pollRef.current = window.setInterval(() => tick().catch(() => {}), 3000);
  }

  if (loading) return <p className="text-slate-400">Загрузка…</p>;
  if (error) return <p className="text-red-400">{error}</p>;

  return (
    <div className="space-y-4">
      <h1 className="text-lg font-semibold">История пачек</h1>
      {batches.length === 0 && <p className="text-slate-500 text-sm">Пачек пока нет.</p>}

      <div className="space-y-3">
        {batches.map((b) => (
          <div key={b.id} className="rounded-xl border border-slate-800 overflow-hidden">
            <button
              onClick={() => toggle(b.id)}
              className="w-full flex items-center gap-4 px-4 py-3 hover:bg-slate-900/50 transition text-left"
            >
              <span className="text-slate-500">{openId === b.id ? "▾" : "▸"}</span>
              <div className="flex-1 min-w-0">
                <div className="font-medium truncate">
                  {b.title || "Без названия"}
                </div>
                <div className="text-xs text-slate-500">
                  {new Date(b.createdAt).toLocaleString("ru-RU")} · модель {b.videoModel}
                </div>
              </div>
              <div className="flex items-center gap-3 text-xs">
                <span className="text-emerald-400">{b.done} готово</span>
                {b.failed > 0 && <span className="text-red-400">{b.failed} ошибок</span>}
                <span className="text-slate-400">из {b.total}</span>
              </div>
            </button>
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
