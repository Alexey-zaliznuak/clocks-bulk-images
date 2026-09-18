export interface VKSettings {
  communityId: number;
  objective: string;
  targetAction: string;
  optimization: boolean;
  biddingStrategy: string;
  budgetDay: number | null;
  budgetTotal: number | null;
  maxPrice: number | null;
  dateStart: string;
  sex: "male" | "female" | "all";
  ageFrom: number;
  ageTo: number;
  ageUnknown: boolean;
  ageRestrictions: string;
  pads: number[];
  refTags: string;
  bannerTitle: string;
  bannerCta: string;
}

export const defaultVKSettings = (): VKSettings => ({
  communityId: 231988974,
  objective: "community",
  targetAction: "send_message",
  optimization: true,
  biddingStrategy: "min_price",
  budgetDay: 999,
  budgetTotal: null,
  maxPrice: null,
  dateStart: "",
  sex: "male",
  ageFrom: 24,
  ageTo: 65,
  ageUnknown: false,
  ageRestrictions: "0+",
  pads: [],
  refTags: "ref_source=vk_ads_yulya&ref={{banner_id}}",
  bannerTitle: "RuTime | именные наручные часы",
  bannerCta: "Узнать цену",
});

export function parseOptionalNumber(value: string): number | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const n = Number(trimmed);
  return Number.isFinite(n) ? n : null;
}

export function parsePadList(value: string): number[] {
  return value
    .split(/[,\s]+/)
    .map((part) => Number(part))
    .filter((n) => Number.isInteger(n) && n > 0);
}
