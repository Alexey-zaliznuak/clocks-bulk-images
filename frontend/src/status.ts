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
      return "bg-emerald-500/15 text-emerald-300 border border-emerald-500/30";
    case "failed":
      return "bg-red-500/15 text-red-300 border border-red-500/30";
    case "queued":
      return "bg-slate-500/15 text-slate-300 border border-slate-500/30";
    default:
      return "bg-sky-500/15 text-sky-300 border border-sky-500/30";
  }
}

export function isActive(status: string): boolean {
  return status !== "done" && status !== "failed";
}
