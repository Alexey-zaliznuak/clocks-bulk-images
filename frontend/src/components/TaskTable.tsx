import { useState } from "react";
import { api, type Task } from "../api";
import { statusClasses, statusLabel } from "../status";
import { formatRub, formatUsd } from "../format";

export default function TaskTable({
  tasks,
  onRetried,
}: {
  tasks: Task[];
  onRetried?: () => void;
}) {
  const [retrying, setRetrying] = useState<string | null>(null);
  const [retryError, setRetryError] = useState("");

  if (tasks.length === 0) {
    return <p className="text-slate-500 text-sm">Пока нет задач.</p>;
  }

  async function retry(id: string) {
    setRetryError("");
    setRetrying(id);
    try {
      await api.retryTask(id);
      onRetried?.();
    } catch (e) {
      setRetryError(e instanceof Error ? e.message : "Не удалось перезапустить задачу");
    } finally {
      setRetrying(null);
    }
  }

  return (
    <div className="space-y-2">
      {retryError && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-xl px-3 py-2">
          {retryError}
        </div>
      )}
      <div className="overflow-x-auto rounded-2xl border border-slate-200 bg-white shadow-sm">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 text-slate-500">
            <tr>
              <th className="text-left font-medium px-4 py-2.5">ФИО</th>
              <th className="text-left font-medium px-4 py-2.5">Статус</th>
              <th className="text-left font-medium px-4 py-2.5">Попытки</th>
              <th className="text-left font-medium px-4 py-2.5">Стоимость</th>
              <th className="text-left font-medium px-4 py-2.5">Изображение</th>
              <th className="text-left font-medium px-4 py-2.5">Видео</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {tasks.map((t) => (
              <tr key={t.id} className="hover:bg-blue-50/40">
                <td className="px-4 py-2.5 whitespace-nowrap font-medium text-slate-800">
                  {t.firstName} {t.lastName}
                </td>
                <td className="px-4 py-2.5">
                  <span
                    className={`inline-block px-2 py-0.5 rounded-full text-xs whitespace-nowrap ${statusClasses(t.status)}`}
                  >
                    {statusLabel(t.status)}
                  </span>
                  {t.error && (
                    <div
                      className={`text-xs mt-1 max-w-xs truncate ${
                        t.status === "failed" ? "text-red-500" : "text-amber-600"
                      }`}
                      title={t.error}
                    >
                      {t.error}
                    </div>
                  )}
                </td>
                <td className="px-4 py-2.5 whitespace-nowrap">
                  {t.attempts > 0 ? (
                    <span className="text-slate-600">{t.attempts}</span>
                  ) : (
                    <span className="text-slate-400">—</span>
                  )}
                  {t.status === "failed" && (
                    <button
                      onClick={() => retry(t.id)}
                      disabled={retrying === t.id}
                      className="block mt-1 text-xs text-blue-600 hover:underline disabled:opacity-50"
                    >
                      {retrying === t.id ? "…" : "повторить"}
                    </button>
                  )}
                </td>
                <td className="px-4 py-2.5 whitespace-nowrap">
                  {t.costUsd > 0 ? (
                    <div className="leading-tight">
                      <div className="font-medium text-slate-800">{formatUsd(t.costUsd)}</div>
                      <div className="text-xs text-slate-500">{formatRub(t.costRub)}</div>
                    </div>
                  ) : (
                    <span className="text-slate-400">—</span>
                  )}
                </td>
                <td className="px-4 py-2.5">
                  {t.imageUrl ? (
                    <a
                      href={t.imageUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="text-blue-600 hover:underline"
                    >
                      открыть
                    </a>
                  ) : (
                    <span className="text-slate-400">—</span>
                  )}
                </td>
                <td className="px-4 py-2.5">
                  {t.videoUrl ? (
                    <a
                      href={t.videoUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 text-emerald-600 hover:underline font-medium"
                    >
                      ▶ скачать
                    </a>
                  ) : (
                    <span className="text-slate-400">—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
