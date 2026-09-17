import { describe, expect, it } from "vitest";
import {
  applyNamePreview,
  hasNamePlaceholder,
  isUnicodeUppercase,
  normalizeCampaignList,
} from "./campaignValidation";

describe("normalizeCampaignList", () => {
  it("обрезает пробелы, пропускает пустые строки и стабильно удаляет дубли", () => {
    expect(normalizeCampaignList([" Анна ", "", "Иван", "Анна", " Иван "])).toEqual({
      values: ["Анна", "Иван"],
      diagnostics: { duplicateCount: 2, invalidCapitalization: [] },
    });
  });

  it("находит элементы со строчной первой Unicode-буквой", () => {
    const result = normalizeCampaignList("анна\nёж\nÉmile\nБорис");
    expect(result.diagnostics.invalidCapitalization).toEqual(["анна", "ёж"]);
  });
});

describe("шаблон рекламного текста", () => {
  it("подставляет имя во все плейсхолдеры", () => {
    expect(applyNamePreview("Для {{name}} от {{name}}", "Анна")).toBe("Для Анна от Анна");
  });

  it("проверяет наличие плейсхолдера", () => {
    expect(hasNamePlaceholder("Привет, {{name}}")).toBe(true);
    expect(hasNamePlaceholder("Привет")).toBe(false);
  });
});

describe("isUnicodeUppercase", () => {
  it("поддерживает кириллицу и отклоняет символы без регистра", () => {
    expect(isUnicodeUppercase("Ёлка")).toBe(true);
    expect(isUnicodeUppercase("ёлка")).toBe(false);
    expect(isUnicodeUppercase("123")).toBe(false);
  });
});
