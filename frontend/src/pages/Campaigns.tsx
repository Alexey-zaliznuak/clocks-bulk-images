import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, type AdCampaign } from "../api";
import { formatRub, formatUsd } from "../format";

export default function Campaigns() {
  const navigate = useNavigate();
  const [campaigns, setCampaigns] = useState<AdCampaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [deletingId, setDeletingId] = useState<string | null>(null);

  useEffect(() => {
    api.listAdCampaigns()
      .then((res) => setCampaigns(res.campaigns || []))
      .catch((e) => setError(e instanceof Error ? e.message : "Не удалось загрузить кампании"))
      .finally(() => setLoading(false));
  }, []);

  async function remove(event: React.MouseEvent, campaign: AdCampaign) {
    event.stopPropagation();
    if (!window.confirm(`Удалить кампанию «${campaign.title}» и все её результаты?`)) return;
    setDeletingId(campaign.id);
    try {
      await api.deleteAdCampaign(campaign.id);
      setCampaigns((current) => current.filter((item) => item.id !== campaign.id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось удалить кампанию");
    } finally {
      setDeletingId(null);
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900">Рекламные кампании</h1>
          <p className="mt-1 text-sm text-slate-500">Массовая генерация роликов для имён и фамилий.</p>
        </div>
        <Link
          to="/campaigns/new"
          className="rounded-xl bg-blue-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-blue-500"
        >
          Создать кампанию
        </Link>
      </div>

      {error && <div className="rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">{error}</div>}
      {loading && <p className="text-sm text-slate-500">Загрузка кампаний…</p>}
      {!loading && campaigns.length === 0 && (
        <div className="rounded-2xl border border-dashed border-slate-300 bg-white px-6 py-10 text-center">
          <p className="font-medium text-slate-800">Кампаний пока нет</p>
          <p className="mt-1 text-sm text-slate-500">Создайте первый черновик и проверьте настройки до запуска.</p>
        </div>
      )}

      <div className="grid gap-3 md:grid-cols-2">
        {campaigns.map((campaign) => {
          const processed = campaign.done + campaign.failed;
          const progress = campaign.total ? Math.round(processed / campaign.total * 100) : 0;
          return (
            <article
              key={campaign.id}
              onClick={() => navigate(`/campaigns/${campaign.id}`)}
              className="cursor-pointer rounded-2xl border border-slate-200 bg-white p-4 shadow-sm transition hover:border-blue-300 hover:shadow-md"
            >
              <div className="flex items-start gap-3">
                <div className="min-w-0 flex-1">
                  <h2 className="truncate font-semibold text-slate-900">{campaign.title}</h2>
                  <p className="mt-0.5 text-xs text-slate-500">
                    {new Date(campaign.createdAt).toLocaleString("ru-RU")}
                  </p>
                </div>
                <span className={`shrink-0 rounded-full border px-2.5 py-1 text-xs font-medium ${lifecycleClasses(campaign.lifecycle)}`}>
                  {lifecycleLabel(campaign.lifecycle)}
                </span>
              </div>

              <div className="mt-4 h-2 overflow-hidden rounded-full bg-slate-100" aria-label={`Прогресс ${progress}%`}>
                <div className="h-full rounded-full bg-blue-600 transition-all" style={{ width: `${progress}%` }} />
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                <span className="text-slate-500">
                  {campaign.nameCount} имён · {campaign.surnameCount} фамилий
                </span>
                <span className="text-emerald-700">{campaign.done} готово</span>
                {campaign.failed > 0 && <span className="text-red-600">{campaign.failed} ошибок</span>}
                <span className="text-slate-500">из {campaign.total}</span>
                <span className="ml-auto text-slate-600" title={formatUsd(campaign.costUsd)}>
                  {formatRub(campaign.costRub)}
                </span>
              </div>
              <div className="mt-4 flex justify-end border-t border-slate-100 pt-3">
                <button
                  type="button"
                  onClick={(event) => void remove(event, campaign)}
                  disabled={deletingId === campaign.id}
                  className="rounded-lg px-3 py-1.5 text-xs font-medium text-slate-500 hover:bg-red-50 hover:text-red-600 disabled:opacity-50"
                >
                  {deletingId === campaign.id ? "Удаляем…" : "Удалить"}
                </button>
              </div>
            </article>
          );
        })}
      </div>
    </div>
  );
}

export function lifecycleLabel(lifecycle: string): string {
  const labels: Record<string, string> = {
    draft: "Черновик",
    running: "Подготовка креативов",
    uploading: "Загрузка в рекламный кабинет",
    completed: "Завершено",
  };
  return labels[lifecycle] || lifecycle;
}

export function lifecycleClasses(lifecycle: string): string {
  if (lifecycle === "draft") return "border-amber-200 bg-amber-50 text-amber-700";
  if (lifecycle === "completed") return "border-emerald-200 bg-emerald-50 text-emerald-700";
  if (lifecycle === "uploading") return "border-violet-200 bg-violet-50 text-violet-700";
  return "border-blue-200 bg-blue-50 text-blue-700";
}
