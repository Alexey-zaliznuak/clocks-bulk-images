// Human-readable labels and colors for the task state machine.

export const STATUS_LABELS: Record<string, string> = {
  queued: "В очереди",
  image_creating: "Создаём изображение",
  image_polling: "Ждём изображение",
  image_ready: "Изображение готово",
  video_creating: "Создаём видео",
  video_polling: "Ждём видео",
  video_downloading: "Сохраняем видео",
  done: "Готово",
  failed: "Ошибка",
};

export function statusLabel(status: string): string {
  return STATUS_LABELS[status] || status;
}

export function statusClasses(status: string): string {
  switch (status) {
    case "done":
      return "bg-emerald-50 text-emerald-700 border border-emerald-200";
    case "failed":
      return "bg-red-50 text-red-700 border border-red-200";
    case "queued":
      return "bg-slate-100 text-slate-600 border border-slate-200";
    default:
      return "bg-blue-50 text-blue-700 border border-blue-200";
  }
}

export function isActive(status: string): boolean {
  return status !== "done" && status !== "failed";
}
