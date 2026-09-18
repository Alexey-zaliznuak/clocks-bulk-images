import { describe, expect, it } from "vitest";
import { vkObjectUrl } from "./vkCabinet";

describe("vkObjectUrl", () => {
  const cabinet = { baseUrl: "https://ads.vk.ru", sudo: "9cfcba08aa@agency_client" };

  it("links a plan with the escaped sudo switch", () => {
    expect(vkObjectUrl(cabinet, { planId: "31656159" })).toBe(
      "https://ads.vk.ru/hq/dashboard/plans/31656159?sudo=9cfcba08aa%40agency_client",
    );
  });

  it("nests a group into its plan", () => {
    expect(vkObjectUrl(cabinet, { planId: "31656159", groupId: "155313486" })).toBe(
      "https://ads.vk.ru/hq/dashboard/plans/31656159/groups/155313486?sudo=9cfcba08aa%40agency_client",
    );
  });

  it("nests an ad into its group", () => {
    expect(vkObjectUrl(cabinet, { planId: "31656159", groupId: "155313486", adId: "238306345" })).toBe(
      "https://ads.vk.ru/hq/dashboard/plans/31656159/groups/155313486/ads/238306345?sudo=9cfcba08aa%40agency_client",
    );
  });

  it("stops at the plan when the group is unknown", () => {
    expect(vkObjectUrl(cabinet, { planId: "31656159", adId: "238306345" })).toBe(
      "https://ads.vk.ru/hq/dashboard/plans/31656159?sudo=9cfcba08aa%40agency_client",
    );
  });

  it("omits the switch when the cabinet reports none", () => {
    expect(vkObjectUrl({ baseUrl: "https://ads.vk.ru/" }, { planId: "1" })).toBe(
      "https://ads.vk.ru/hq/dashboard/plans/1",
    );
  });

  it("returns nothing without a cabinet or a plan", () => {
    expect(vkObjectUrl(null, { planId: "1" })).toBe("");
    expect(vkObjectUrl(cabinet, { groupId: "155313486" })).toBe("");
  });
});
