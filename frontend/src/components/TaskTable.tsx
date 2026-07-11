import type { Task } from "../api";
import { statusClasses, statusLabel } from "../status";

export default function TaskTable({ tasks }: { tasks: Task[] }) {
  if (tasks.length === 0) {
    return <p className="text-slate-500 text-sm">Пока нет задач.</p>;
  }
  return (
    <div className="overflow-x-auto rounded-xl border border-slate-800">
      <table className="w-full text-sm">
        <thead className="bg-slate-900/70 text-slate-400">
          <tr>
            <th className="text-left font-medium px-4 py-2.5">ФИО</th>
            <th className="text-left font-medium px-4 py-2.5">Статус</th>
            <th className="text-left font-medium px-4 py-2.5">Изображение</th>
            <th className="text-left font-medium px-4 py-2.5">Видео</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-800">
          {tasks.map((t) => (
            <tr key={t.id} className="hover:bg-slate-900/40">
              <td className="px-4 py-2.5 whitespace-nowrap">
                {t.firstName} {t.lastName}
              </td>
              <td className="px-4 py-2.5">
                <span className={`inline-block px-2 py-0.5 rounded-full text-xs ${statusClasses(t.status)}`}>
                  {statusLabel(t.status)}
                </span>
                {t.status === "failed" && t.error && (
                  <div className="text-xs text-red-400/80 mt-1 max-w-xs truncate" title={t.error}>
                    {t.error}
                  </div>
                )}
              </td>
              <td className="px-4 py-2.5">
                {t.imageUrl ? (
                  <a
                    href={t.imageUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="text-sky-400 hover:underline"
                  >
                    открыть
                  </a>
                ) : (
                  <span className="text-slate-600">—</span>
                )}
              </td>
              <td className="px-4 py-2.5">
                {t.videoUrl ? (
                  <a
                    href={t.videoUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 text-emerald-400 hover:underline font-medium"
                  >
                    ▶ скачать
                  </a>
                ) : (
                  <span className="text-slate-600">—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
