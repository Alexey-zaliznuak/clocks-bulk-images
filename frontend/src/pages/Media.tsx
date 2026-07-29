import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api, type MediaAsset } from "../api";
import ConfirmDialog from "../components/ConfirmDialog";
import { formatBytes, formatDuration } from "../format";

export default function Media() {
  const [assets, setAssets] = useState<MediaAsset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [uploading, setUploading] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<MediaAsset | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  async function load() {
    try {
      const res = await api.listAudio();
      setAssets(res.assets || []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось загрузить медиатеку");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function uploadFiles(files: FileList | File[]) {
    setError("");
    setUploading(true);
    try {
      for (const file of Array.from(files)) {
        await api.uploadAudio(file);
      }
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось загрузить файл");
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  async function remove(asset: MediaAsset) {
    setError("");
    try {
      await api.deleteAudio(asset.id);
      setAssets((list) => list.filter((a) => a.id !== asset.id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось удалить файл");
    }
  }

  return (
    <div className="max-w-3xl mx-auto space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900 mb-1">Медиа 🎵</h1>
        <p className="text-sm text-slate-500">
          Библиотека mp3 для озвучки видео. При создании пачки выбирается один трек — видео
          подгоняется точно под его длительность. Нужно вытащить звук из готового видео?{" "}
          <Link to="/mp4-to-mp3" className="text-blue-600 hover:underline">
            MP4 → MP3
          </Link>
          .
        </p>
      </div>

      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          if (e.dataTransfer.files?.length) uploadFiles(e.dataTransfer.files);
        }}
        className={`rounded-2xl border-2 border-dashed px-6 py-8 text-center transition ${
          dragging ? "border-blue-400 bg-blue-50" : "border-slate-300 bg-white"
        }`}
      >
        <p className="text-sm text-slate-600">
          Перетащите mp3 сюда или{" "}
          <button
            onClick={() => fileRef.current?.click()}
            disabled={uploading}
            className="text-blue-600 hover:underline disabled:opacity-50"
          >
            выберите файлы
          </button>
        </p>
        <p className="text-xs text-slate-400 mt-1">
          {uploading ? "Загружаем…" : "Только .mp3, до 50 МБ"}
        </p>
        <input
          ref={fileRef}
          type="file"
          accept="audio/mpeg,.mp3"
          multiple
          className="hidden"
          onChange={(e) => e.target.files?.length && uploadFiles(e.target.files)}
        />
      </div>

      {error && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-xl px-3 py-2">
          {error}
        </div>
      )}

      {loading ? (
        <p className="text-slate-500 text-sm">Загрузка…</p>
      ) : assets.length === 0 ? (
        <p className="text-slate-500 text-sm">Пока нет загруженных треков.</p>
      ) : (
        <ul className="space-y-2">
          {assets.map((a) => (
            <li
              key={a.id}
              className="rounded-2xl border border-slate-200 bg-white shadow-sm px-4 py-3 space-y-2"
            >
              <div className="flex items-start gap-3">
                <div className="flex-1 min-w-0">
                  <p className="font-medium text-slate-800 truncate" title={a.filename}>
                    {a.title || a.filename}
                  </p>
                  <p className="text-xs text-slate-500">
                    {formatDuration(a.durationSeconds)} · {formatBytes(a.sizeBytes)} ·{" "}
                    {new Date(a.createdAt).toLocaleString("ru-RU")}
                  </p>
                </div>
                {a.url && (
                  <a
                    href={a.url}
                    download={a.filename}
                    className="text-sm text-emerald-600 hover:underline whitespace-nowrap"
                  >
                    ↓ скачать
                  </a>
                )}
                <button
                  onClick={() => setPendingDelete(a)}
                  className="text-sm text-slate-400 hover:text-red-600 transition"
                  title="Удалить"
                >
                  🗑
                </button>
              </div>
              {a.url && <audio controls preload="none" src={a.url} className="w-full h-9" />}
            </li>
          ))}
        </ul>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        title="Удалить трек?"
        confirmLabel="Удалить"
        tone="danger"
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => {
          const asset = pendingDelete;
          setPendingDelete(null);
          if (asset) remove(asset);
        }}
      >
        <p>
          «{pendingDelete?.title || pendingDelete?.filename}» будет удалён из медиатеки
          безвозвратно. Уже готовые видео это не затронет.
        </p>
      </ConfirmDialog>
    </div>
  );
}
