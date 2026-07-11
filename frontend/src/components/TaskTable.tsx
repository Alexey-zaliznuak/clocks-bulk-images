import type { Task } from "../api";
import { statusClasses, statusLabel } from "../status";
import { formatRub, formatUsd } from "../format";

export default function TaskTable({ tasks }: { tasks: Task[] }) {
  if (tasks.length === 0) {
    return <p className="text-slate-500 text-sm">Пока нет задач.</p>;
  }
  return (
    <div className="overflow-x-auto rounded-2xl border border-slate-200 bg-white shadow-sm">
      <table className="w-full text-sm">
        <thead className="bg-slate-50 text-slate-500">
          <tr>
            <th className="text-left font-medium px-4 py-2.5">ФИО</th>
            <th className="text-left font-medium px-4 py-2.5">Статус</th>
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
                <span className={`inline-block px-2 py-0.5 rounded-full text-xs ${statusClasses(t.status)}`}>
                  {statusLabel(t.status)}
                </span>
                {t.status === "failed" && t.error && (
                  <div className="text-xs text-red-500 mt-1 max-w-xs truncate" title={t.error}>
                    {t.error}
                  </div>
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
  );
}
