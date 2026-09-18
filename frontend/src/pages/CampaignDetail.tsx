import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type AdCampaign, type AdCampaignItem } from "../api";
import ConfirmDialog from "../components/ConfirmDialog";
import CampaignSettingsView from "../components/CampaignSettingsView";
import { formatRub, formatUsd } from "../format";
import { type PadNode } from "../padsTree";
import { statusClasses, statusLabel } from "../status";
import { isVKUpload, lifecycleClasses, lifecycleLabel } from "./Campaigns";

type KindFilter = "" | "name" | "surname";
const PAGE_SIZE = 50;

export default function CampaignDetail() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [campaign, setCampaign] = useState<AdCampaign | null>(null);
  const [items, setItems] = useState<AdCampaignItem[]>([]);
  const [total, setTotal] = useState(0);
  const [kind, setKind] = useState<KindFilter>("");
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [itemsLoading, setItemsLoading] = useState(true);
  const [error, setError] = useState("");
  const [itemsError, setItemsError] = useState("");
  const [startOpen, setStartOpen] = useState(false);
  const [starting, setStarting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [retryingAll, setRetryingAll] = useState(false);
  const [retryingItem, setRetryingItem] = useState<string | null>(null);
  const [ignoreOpen, setIgnoreOpen] = useState(false);
  const [ignoring, setIgnoring] = useState(false);
  const [message, setMessage] = useState("");
  const [padTrees, setPadTrees] = useState<PadNode[]>([]);

  const loadCampaign = useCallback(async () => {
    try {
      const res = await api.getAdCampaign(id);
      setCampaign(res.campaign);
      setError("");
      return res.campaign;
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось загрузить кампанию");
      return null;
    } finally {
      setLoading(false);
    }
  }, [id]);

  const loadItems = useCallback(async () => {
    setItemsLoading(true);
    try {
      const res = await api.listAdCampaignItems(id, {
        limit: PAGE_SIZE,
        offset,
        kind: kind || undefined,
      });
      setItems(res.items);
      setTotal(res.total);
      setItemsError("");
    } catch (e) {
      setItemsError(e instanceof Error ? e.message : "Не удалось загрузить задачи");
    } finally {
      setItemsLoading(false);
    }
  }, [id, kind, offset]);

  useEffect(() => {
    void loadCampaign();
    void loadItems();
    api.vkAdsPads()
      .then((res) => setPadTrees(res.trees || []))
      .catch(() => {});
  }, [loadCampaign, loadItems]);

  useEffect(() => {
    if (campaign?.lifecycle !== "running" && !isVKUpload(campaign?.lifecycle || "")) return;
    const timer = window.setInterval(() => {
      void loadCampaign();
      void loadItems();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [campaign?.lifecycle, loadCampaign, loadItems]);

  function selectKind(next: KindFilter) {
    setKind(next);
    setOffset(0);
  }

  async function start() {
    setStarting(true);
    setError("");
    try {
      const result = await api.startAdCampaign(id);
      setMessage(`Поставлено в очередь: ${result.queued}`);
      setStartOpen(false);
      await Promise.all([loadCampaign(), loadItems()]);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось запустить кампанию");
    } finally {
      setStarting(false);
    }
  }

  async function ignoreFailed() {
    setIgnoring(true);
    setError("");
    try {
      await api.ignoreAdCampaignFailures(id);
      setMessage("Ошибки проигнорированы — загружаем в ВКР только успешные задачи");
      setIgnoreOpen(false);
      await Promise.all([loadCampaign(), loadItems()]);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось проигнорировать ошибки");
    } finally {
      setIgnoring(false);
    }
  }

  async function retryAll() {
    setRetryingAll(true);
    setError("");
    try {
      const result = await api.retryAdCampaign(id);
      setMessage(`Перезапущено задач: ${result.retried}`);
      await Promise.all([loadCampaign(), loadItems()]);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось повторить ошибки");
    } finally {
      setRetryingAll(false);
    }
  }

  async function retryItem(itemId: string) {
    setRetryingItem(itemId);
    setItemsError("");
    try {
      await api.retryAdCampaignItem(itemId);
      await Promise.all([loadCampaign(), loadItems()]);
    } catch (e) {
      setItemsError(e instanceof Error ? e.message : "Не удалось повторить задачу");
    } finally {
      setRetryingItem(null);
    }
  }

  async function remove() {
    if (!campaign || !window.confirm(`Удалить кампанию «${campaign.title}» и все её результаты?`)) return;
    setDeleting(true);
    try {
      await api.deleteAdCampaign(id);
      navigate("/campaigns");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось удалить кампанию");
      setDeleting(false);
    }
  }

  if (loading && !campaign) return <p className="text-sm text-slate-500">Загрузка кампании…</p>;
  if (!campaign) {
    return (
      <div className="space-y-3">
        {error && <ErrorBox>{error}</ErrorBox>}
        <Link to="/campaigns" className="text-sm text-blue-600 hover:underline">К списку кампаний</Link>
      </div>
    );
  }

  const allTerminal = campaign.total > 0 && campaign.done + campaign.failed === campaign.total;
  const waitingToIgnore =
    allTerminal
    && campaign.failed > 0
    && campaign.done > 0
    && !campaign.vkIgnoreFailed
    && campaign.lifecycle !== "completed"
    && campaign.lifecycle !== "draft";
  const showUploading = isVKUpload(campaign.lifecycle) && !waitingToIgnore;
  const canRetry = campaign.lifecycle === "running" && !campaign.vkIgnoreFailed && !campaign.vkAdPlanId;

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-center gap-3">
        <Link to="/campaigns" className="text-sm text-slate-500 hover:text-blue-600">← К кампаниям</Link>
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-xl font-bold text-slate-900">{campaign.title}</h1>
          <p className="mt-0.5 text-xs text-slate-500">{new Date(campaign.createdAt).toLocaleString("ru-RU")}</p>
        </div>
        <span className={`rounded-full border px-2.5 py-1 text-xs font-medium ${lifecycleClasses(campaign.lifecycle)}`}>
          {lifecycleLabel(campaign.lifecycle)}
        </span>
        <button
          type="button"
          onClick={() => void remove()}
          disabled={deleting}
          className="rounded-xl px-3 py-2 text-sm font-medium text-slate-500 hover:bg-red-50 hover:text-red-600 disabled:opacity-50"
        >
          {deleting ? "Удаляем…" : "Удалить"}
        </button>
      </header>

      {error && <ErrorBox>{error}</ErrorBox>}
      {message && <div className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{message}</div>}

      {showUploading && (
        <section className="rounded-2xl border border-violet-200 bg-violet-50 p-5">
          <h2 className="font-semibold text-violet-900">{lifecycleLabel(campaign.lifecycle)}</h2>
          <p className="mt-1 text-sm text-violet-800">
            {campaign.lifecycle === "vk_groups"
              ? "Создаём группы объявлений в кабинете VK Рекламы."
              : campaign.lifecycle === "vk_ads"
                ? "Загружаем видео и регистрируем объявления в группах."
                : "Регистрируем кампанию в кабинете VK Рекламы."}
          </p>
        </section>
      )}

      {waitingToIgnore && (
        <section className="rounded-2xl border border-amber-300 bg-amber-50 p-5">
          <h2 className="font-semibold text-amber-900">Генерация закончилась с ошибками</h2>
          <p className="mt-1 text-sm text-amber-800">
            В кабинет автоматически уйдут только кампании, где все имена и фамилии успешны.
            Можно проигнорировать {campaign.failed} {campaign.failed === 1 ? "ошибку" : "ошибок"} и загрузить {campaign.done} готовых задач.
          </p>
          <button
            type="button"
            onClick={() => setIgnoreOpen(true)}
            className="mt-4 rounded-xl bg-amber-600 px-4 py-2 text-sm font-semibold text-white hover:bg-amber-500"
          >
            Игнорировать ошибки и загрузить в ВКР
          </button>
        </section>
      )}

      {campaign.lifecycle === "draft" && (
        <section className="rounded-2xl border border-amber-300 bg-amber-50 p-5">
          <h2 className="font-semibold text-amber-900">Черновик готов к проверке</h2>
          <p className="mt-1 text-sm text-amber-800">
            {campaign.nameCount} имён + {campaign.surnameCount} фамилий = {campaign.total} платных задач.
            Генерация ещё не запускалась.
          </p>
          <button
            type="button"
            onClick={() => setStartOpen(true)}
            className="mt-4 rounded-xl bg-amber-600 px-4 py-2 text-sm font-semibold text-white hover:bg-amber-500"
          >
            Запустить кампанию
          </button>
        </section>
      )}

      <section className="grid gap-3 sm:grid-cols-3">
        <SummaryCard label="Готово" value={`${campaign.done}/${campaign.total}`} tone="success" />
        <SummaryCard label="Ошибки" value={String(campaign.failed)} tone={campaign.failed ? "error" : "default"} />
        <SummaryCard label="Стоимость" value={formatRub(campaign.costRub)} hint={formatUsd(campaign.costUsd)} tone="default" />
      </section>

      <CampaignSettingsView
        campaign={campaign}
        padTrees={padTrees}
      />

      <section className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold text-slate-900">Задачи</h2>
            <p className="text-xs text-slate-500">Показано {items.length} из {total}</p>
          </div>
          {canRetry && campaign.failed > 0 && (
            <button
              type="button"
              onClick={() => void retryAll()}
              disabled={retryingAll}
              className="rounded-xl bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
            >
              {retryingAll ? "Повторяем…" : `Повторить ошибки (${campaign.failed})`}
            </button>
          )}
        </div>

        <div className="flex overflow-x-auto border-b border-slate-200" aria-label="Фильтр задач">
          <FilterButton active={kind === ""} onClick={() => selectKind("")}>Все</FilterButton>
          <FilterButton active={kind === "name"} onClick={() => selectKind("name")}>Имена</FilterButton>
          <FilterButton active={kind === "surname"} onClick={() => selectKind("surname")}>Фамилии</FilterButton>
        </div>

        {itemsError && <ErrorBox>{itemsError}</ErrorBox>}
        {itemsLoading && items.length === 0 ? (
          <p className="text-sm text-slate-500">Загрузка задач…</p>
        ) : items.length === 0 ? (
          <p className="rounded-xl border border-dashed border-slate-300 bg-white px-4 py-8 text-center text-sm text-slate-500">
            По выбранному фильтру задач нет.
          </p>
        ) : (
          <div className="overflow-x-auto rounded-2xl border border-slate-200 bg-white shadow-sm">
            <table className="w-full min-w-[900px] text-sm">
              <thead className="bg-slate-50 text-slate-500">
                <tr>
                  <th className={thCls}>Элемент</th>
                  <th className={thCls}>Статус</th>
                  <th className={thCls}>Аудитория</th>
                  <th className={thCls}>Группа ВКР</th>
                  <th className={thCls}>Объявление ВКР</th>
                  <th className={thCls}>Картинка</th>
                  <th className={thCls}>Исходное видео</th>
                  <th className={thCls}>Преобразованное видео</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {items.map((item) => (
                  <tr key={item.id} className="hover:bg-blue-50/40">
                    <td className="px-4 py-3">
                      <div className="font-medium text-slate-800">{item.value}</div>
                      <span className="mt-1 inline-block rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500">
                        {item.kind === "name" ? "Имя" : "Фамилия"}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`inline-block whitespace-nowrap rounded-full px-2 py-0.5 text-xs ${statusClasses(item.status)}`}>
                        {statusLabel(item.status)}
                      </span>
                      {item.error && <p className="mt-1 max-w-xs text-xs text-red-600" title={item.error}>{item.error}</p>}
                      {item.status === "failed" && canRetry && (
                        <button
                          type="button"
                          onClick={() => void retryItem(item.id)}
                          disabled={retryingItem === item.id}
                          className="mt-1 block text-xs font-medium text-blue-600 hover:underline disabled:opacity-50"
                        >
                          {retryingItem === item.id ? "Повторяем…" : "Повторить"}
                        </button>
                      )}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-600">
                      {item.audienceName || (item.audienceId ? String(item.audienceId) : "—")}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-600">{item.vkAdGroupId || "—"}</td>
                    <td className="px-4 py-3 text-xs text-slate-600">{item.vkBannerId || "—"}</td>
                    <DownloadCell url={item.imageDownloadUrl} label="Скачать картинку" />
                    <DownloadCell url={item.sourceDownloadUrl} label="Скачать исходное видео" />
                    <DownloadCell url={item.videoDownloadUrl} label="Скачать преобразованное видео" />
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {total > PAGE_SIZE && (
          <div className="flex items-center justify-between gap-3">
            <button
              type="button"
              disabled={offset === 0}
              onClick={() => setOffset((value) => Math.max(0, value - PAGE_SIZE))}
              className={pageButtonCls}
            >
              Назад
            </button>
            <span className="text-xs text-slate-500">
              {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} из {total}
            </span>
            <button
              type="button"
              disabled={offset + PAGE_SIZE >= total}
              onClick={() => setOffset((value) => value + PAGE_SIZE)}
              className={pageButtonCls}
            >
              Далее
            </button>
          </div>
        )}
      </section>

      <ConfirmDialog
        open={startOpen}
        title="Запустить платную генерацию?"
        confirmLabel={starting ? "Запускаем…" : `Запустить ${campaign.total} задач`}
        onCancel={() => !starting && setStartOpen(false)}
        onConfirm={() => void start()}
      >
        <p>
          Будут запущены {campaign.total} платных задач. После запуска изменить настройки кампании нельзя.
        </p>
      </ConfirmDialog>
      <ConfirmDialog
        open={ignoreOpen}
        title="Игнорировать ошибки и загрузить в ВКР?"
        confirmLabel={ignoring ? "Загружаем…" : "Игнорировать и загрузить"}
        onCancel={() => !ignoring && setIgnoreOpen(false)}
        onConfirm={() => void ignoreFailed()}
      >
        <p>
          В кабинет уйдут только {campaign.done} успешных задач. {campaign.failed} с ошибками останутся без групп.
        </p>
      </ConfirmDialog>
    </div>
  );
}

function DownloadCell({ url, label }: { url?: string; label: string }) {
  return (
    <td className="px-4 py-3">
      {url ? (
        <a href={url} download className="whitespace-nowrap font-medium text-blue-600 hover:underline">{label}</a>
      ) : (
        <span className="text-slate-400">—</span>
      )}
    </td>
  );
}

function SummaryCard({ label, value, hint, tone }: { label: string; value: string; hint?: string; tone: "success" | "error" | "default" }) {
  const valueClass = tone === "success" ? "text-emerald-700" : tone === "error" ? "text-red-600" : "text-slate-900";
  return (
    <div className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
      <p className="text-xs text-slate-500">{label}</p>
      <p className={`mt-1 text-xl font-bold ${valueClass}`}>{value}</p>
      {hint && <p className="mt-0.5 text-xs text-slate-500">{hint}</p>}
    </div>
  );
}

function FilterButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`shrink-0 border-b-2 px-4 py-2 text-sm font-medium ${
        active ? "border-blue-600 text-blue-700" : "border-transparent text-slate-500 hover:text-slate-800"
      }`}
    >
      {children}
    </button>
  );
}

function ErrorBox({ children }: { children: React.ReactNode }) {
  return <div className="rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600">{children}</div>;
}

const thCls = "px-4 py-2.5 text-left font-medium";
const pageButtonCls = "rounded-xl border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:border-blue-300 disabled:opacity-40";
