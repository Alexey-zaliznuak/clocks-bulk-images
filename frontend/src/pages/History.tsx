import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, type Batch } from "../api";
import { formatRub, formatUsd } from "../format";

export default function History() {
  const navigate = useNavigate();
  const [batches, setBatches] = useState<Batch[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deletingId, setDeletingId] = useState<string | null>(null);

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
  }, []);

  async function remove(e: React.MouseEvent, batchId: string) {
    e.stopPropagation();
    if (!window.confirm("Удалить эту пачку и все её видео безвозвратно?")) return;
    setDeletingId(batchId);
    try {
      await api.deleteBatch(batchId);
      setBatches((prev) => prev.filter((b) => b.id !== batchId));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось удалить пачку");
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
            onClick={() => navigate(`/batch/${b.id}`)}
            className="flex items-center gap-2 pr-2 rounded-2xl border border-slate-200 bg-white shadow-sm hover:border-blue-300 hover:shadow-md cursor-pointer transition"
          >
            <div className="flex-1 flex items-center gap-4 px-4 py-3 min-w-0">
              <span className="text-blue-500">▸</span>
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
                  <span className="text-slate-600" title={`${formatUsd(b.costUsd)} по текущему курсу`}>
                    {formatRub(b.costRub)}
                  </span>
                )}
                <span className="text-emerald-600">{b.done} готово</span>
                {b.failed > 0 && <span className="text-red-500">{b.failed} ошибок</span>}
                <span className="text-slate-400">из {b.total}</span>
              </div>
            </div>
            <button
              onClick={(e) => remove(e, b.id)}
              disabled={deletingId === b.id}
              title="Удалить пачку"
              className="shrink-0 px-2.5 py-1.5 rounded-lg text-sm text-slate-400 hover:text-red-600 hover:bg-red-50 disabled:opacity-50 transition"
            >
              {deletingId === b.id ? "…" : "🗑"}
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}
