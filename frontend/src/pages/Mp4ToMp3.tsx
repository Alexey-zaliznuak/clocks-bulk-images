import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api";
import { formatBytes } from "../format";

// Mp4ToMp3 extracts the soundtrack of a video the user uploads and hands the mp3
// straight back — nothing is stored on the server.
export default function Mp4ToMp3() {
  const [file, setFile] = useState<File | null>(null);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState("");
  const [dragging, setDragging] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  function pick(files: FileList | null) {
    setError("");
    setDone("");
    setFile(files && files.length > 0 ? files[0] : null);
  }

  async function convert() {
    if (!file) return;
    setError("");
    setDone("");
    setWorking(true);
    try {
      const { blob, filename } = await api.extractAudio(file);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      setDone(`Готово: ${filename} (${formatBytes(blob.size)})`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось извлечь аудио");
    } finally {
      setWorking(false);
    }
  }

  return (
    <div className="max-w-xl mx-auto space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900 mb-1">MP4 → MP3 🎬</h1>
        <p className="text-sm text-slate-500">
          Загрузите видео — сервер вытащит из него звуковую дорожку и вернёт mp3. Файл на
          сервере не сохраняется; чтобы использовать трек для озвучки, загрузите его в{" "}
          <Link to="/media" className="text-blue-600 hover:underline">
            Медиа
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
          pick(e.dataTransfer.files);
        }}
        className={`rounded-2xl border-2 border-dashed px-6 py-8 text-center transition ${
          dragging ? "border-blue-400 bg-blue-50" : "border-slate-300 bg-white"
        }`}
      >
        {file ? (
          <div className="space-y-1">
            <p className="font-medium text-slate-800 truncate" title={file.name}>
              {file.name}
            </p>
            <p className="text-xs text-slate-500">{formatBytes(file.size)}</p>
            <button
              onClick={() => pick(null)}
              className="text-xs text-slate-500 hover:text-red-600 transition"
            >
              убрать
            </button>
          </div>
        ) : (
          <>
            <p className="text-sm text-slate-600">
              Перетащите mp4 сюда или{" "}
              <button
                onClick={() => fileRef.current?.click()}
                className="text-blue-600 hover:underline"
              >
                выберите файл
              </button>
            </p>
            <p className="text-xs text-slate-400 mt-1">До 500 МБ</p>
          </>
        )}
        <input
          ref={fileRef}
          type="file"
          accept="video/mp4,video/*,.mp4,.mov,.mkv,.webm"
          className="hidden"
          onChange={(e) => pick(e.target.files)}
        />
      </div>

      {error && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-xl px-3 py-2">
          {error}
        </div>
      )}
      {done && (
        <div className="text-sm text-emerald-700 bg-emerald-50 border border-emerald-200 rounded-xl px-3 py-2">
          {done}
        </div>
      )}

      <button
        onClick={convert}
        disabled={!file || working}
        className="w-full py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-semibold shadow-sm transition"
      >
        {working ? "Извлекаем звук…" : "Скачать mp3"}
      </button>
    </div>
  );
}
