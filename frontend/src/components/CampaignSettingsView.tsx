import { useState } from "react";
import { type AdCampaign } from "../api";
import PadsTree from "./PadsTree";
import VKLink from "./VKLink";
import { type PadNode } from "../padsTree";
import { type VKCabinet } from "../vkCabinet";
import { defaultVKSettings } from "../vkSettings";

type SettingsTab = "campaign" | "group" | "ads" | "generation";
type ListTab = "names" | "surnames";

export default function CampaignSettingsView({
  campaign,
  padTrees,
  padsLoading,
  padsError,
  cabinet = null,
}: {
  campaign: AdCampaign;
  padTrees: PadNode[];
  padsLoading?: boolean;
  padsError?: string;
  cabinet?: VKCabinet | null;
}) {
  const [tab, setTab] = useState<SettingsTab>("campaign");
  const [listTab, setListTab] = useState<ListTab>("names");
  const [adsTab, setAdsTab] = useState<ListTab>("names");
  const vk = { ...defaultVKSettings(), ...campaign.vkSettings };
  const names = campaign.names || [];
  const surnames = campaign.surnames || [];

  return (
    <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
      <div className="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">Параметры кампании — только просмотр</h2>
      </div>
      <div className="flex overflow-x-auto border-b border-slate-200" role="tablist" aria-label="Разделы настроек">
        <TabButton active={tab === "campaign"} onClick={() => setTab("campaign")}>Настройки кампании</TabButton>
        <TabButton active={tab === "group"} onClick={() => setTab("group")}>Группа</TabButton>
        <TabButton active={tab === "ads"} onClick={() => setTab("ads")}>Объявления</TabButton>
        <TabButton active={tab === "generation"} onClick={() => setTab("generation")}>Генерация</TabButton>
      </div>
      <div className="space-y-4 p-4">
        {tab === "campaign" && (
          <>
            <Field label="Название кампании" value={campaign.title} />
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="ID сообщества" value={String(vk.communityId || "—")} />
              <Field label="Целевое действие" value={vk.targetAction === "send_message" ? "Отправка сообщения" : vk.targetAction || "—"} />
            </div>
            <Check label="Оптимизация бюджета включена" checked={vk.optimization} />
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Стратегия ставок" value={biddingLabel(vk.biddingStrategy)} />
              <Field label="Дневной бюджет, ₽" value={vk.budgetDay == null ? "не задан" : String(vk.budgetDay)} />
              <Field label="Общий бюджет, ₽" value={vk.budgetTotal == null ? "не задан" : String(vk.budgetTotal)} />
              <Field label="Ограничение ставки, ₽" value={vk.maxPrice == null ? "не задано" : String(vk.maxPrice)} />
            </div>
            <Field label="REF-метки" value={vk.refTags || "—"} />
            <div>
              <span className="mb-1 block text-xs text-slate-500">ID кампании ВКР</span>
              <VKLink
                cabinet={cabinet}
                planId={campaign.vkAdPlanId}
                fallback="ещё не создана"
                className="text-sm"
              />
            </div>
            {campaign.vkUploadError && <Field label="Ошибка загрузки в ВКР" value={campaign.vkUploadError} multiline />}
            <p className="text-xs text-slate-500">Дата показа — с даты создания кампании, без даты окончания.</p>
            <hr className="border-slate-200" />
            <div className="flex overflow-x-auto border-b border-slate-200" role="tablist" aria-label="Списки">
              <TabButton active={listTab === "names"} onClick={() => setListTab("names")}>Имена ({names.length})</TabButton>
              <TabButton active={listTab === "surnames"} onClick={() => setListTab("surnames")}>Фамилии ({surnames.length})</TabButton>
            </div>
            <Field
              label={listTab === "names" ? `Список имён (${names.length})` : `Список фамилий (${surnames.length})`}
              value={(listTab === "names" ? names : surnames).join("\n") || "—"}
              multiline
            />
          </>
        )}

        {tab === "group" && (
          <>
            <p className="text-sm text-slate-500">
              Эти настройки клонируются во все группы ВКР. На каждое имя и фамилию будет своя группа.
            </p>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="Пол" value={sexLabel(vk.sex)} />
              <Field label="Маркировка" value={vk.ageRestrictions || "—"} />
              <Field label="Возраст от" value={String(vk.ageFrom || "—")} />
              <Field label="Возраст до" value={String(vk.ageTo || "—")} />
            </div>
            <Check label="Включать возраст «не определён»" checked={vk.ageUnknown} />
            <div>
              <span className="mb-1 block text-sm text-slate-500">Места размещения</span>
              <PadsTree
                trees={padTrees}
                selected={vk.pads || []}
                loading={padsLoading}
                error={padsError}
                readOnly
              />
              <p className="mt-1 text-xs text-slate-500">
                {vk.pads?.length ? `Выбрано площадок: ${vk.pads.length}` : "Ничего не выбрано — при загрузке возьмём ленту ВК."}
              </p>
            </div>
            <p className="text-xs text-slate-500">
              Время 6–21, регион Россия, расширение аудитории выключено — эти поля не настраиваются.
            </p>
          </>
        )}

        {tab === "ads" && (
          <>
            <Field label="Заголовок объявления" value={vk.bannerTitle || "—"} />
            <Field label="Надпись на кнопке" value={vk.bannerCta || "—"} />
            <div className="flex overflow-x-auto border-b border-slate-200" role="tablist" aria-label="Описание объявления">
              <TabButton active={adsTab === "names"} onClick={() => setAdsTab("names")}>Описание для имён</TabButton>
              <TabButton active={adsTab === "surnames"} onClick={() => setAdsTab("surnames")}>Описание для фамилий</TabButton>
            </div>
            <Field
              label="Описание (используйте {{name}})"
              value={(adsTab === "names" ? campaign.nameTextTemplate : campaign.surnameTextTemplate) || "—"}
              multiline
            />
          </>
        )}

        {tab === "generation" && (
          <>
            <Field label="ID шаблона Иманатора" value={campaign.templateId || "—"} />
            <Field label="Ключ подстановки имени" value={campaign.nameSettingKey || "—"} />
            <Field label="Доп. настройки изображения (JSON)" value={JSON.stringify(campaign.imageSettings || {}, null, 2)} multiline />
            <hr className="border-slate-200" />
            <Field label="Модель видео (OpenRouter)" value={campaign.videoModel || "—"} />
            <Field label="Промпт для видео" value={campaign.videoPrompt || "—"} multiline />
            <div className="grid gap-3 sm:grid-cols-3">
              <Field label="Длительность (сек)" value={campaign.videoDuration == null ? "авто" : String(campaign.videoDuration)} />
              <Field label="Разрешение" value={campaign.videoResolution || "авто"} />
              <Field label="Соотношение" value={campaign.videoAspectRatio || "авто"} />
            </div>
            <hr className="border-slate-200" />
            <Field
              label="Музыка (mp3 из медиатеки)"
              value={campaign.generateAudio ? "не используется — звук от модели" : (campaign.audioAssetId || "—")}
            />
            <Check label="Генерировать аудио моделью" checked={campaign.generateAudio} />
          </>
        )}
      </div>
    </section>
  );
}

function Field({ label, value, multiline }: { label: string; value: string; multiline?: boolean }) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm text-slate-500">{label}</span>
      {multiline ? (
        <textarea className={`${readCls} min-h-32 font-mono text-sm`} value={value} readOnly />
      ) : (
        <input className={readCls} value={value} readOnly />
      )}
    </label>
  );
}

function Check({ label, checked }: { label: string; checked: boolean }) {
  return (
    <label className="flex items-start gap-2.5">
      <input type="checkbox" className="mt-0.5 h-4 w-4" checked={checked} disabled readOnly />
      <span className="text-sm text-slate-700">{label}</span>
    </label>
  );
}

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`shrink-0 border-b-2 px-5 py-3 text-sm font-medium transition ${
        active ? "border-blue-600 text-blue-700" : "border-transparent text-slate-500 hover:text-slate-800"
      }`}
    >
      {children}
    </button>
  );
}

function biddingLabel(value: string) {
  if (value === "min_price") return "Минимальная цена";
  if (value === "max_goals") return "Максимум конверсий";
  if (value === "second_price_mean") return "Средняя цена";
  return value || "—";
}

function sexLabel(value: string) {
  if (value === "female") return "Женский";
  if (value === "all") return "Все";
  return "Мужской";
}

const readCls = "w-full cursor-not-allowed rounded-xl border border-slate-200 bg-slate-100 px-3 py-2 text-sm text-slate-600";
