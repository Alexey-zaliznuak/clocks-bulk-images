export interface ListDiagnostics {
  duplicateCount: number;
  invalidCapitalization: string[];
}

export interface NormalizedList {
  values: string[];
  diagnostics: ListDiagnostics;
}

export function isUnicodeUppercase(value: string): boolean {
  const char = Array.from(value.trim())[0];
  if (!char) return false;
  return (
    char === char.toLocaleUpperCase("ru-RU") &&
    char !== char.toLocaleLowerCase("ru-RU")
  );
}

export function normalizeCampaignList(input: string[] | string): NormalizedList {
  const source = typeof input === "string" ? input.split(/\r?\n/) : input;
  const values: string[] = [];
  const seen = new Set<string>();
  let duplicateCount = 0;

  for (const raw of source) {
    const value = raw.trim();
    if (!value) continue;
    if (seen.has(value)) {
      duplicateCount++;
      continue;
    }
    seen.add(value);
    values.push(value);
  }

  return {
    values,
    diagnostics: {
      duplicateCount,
      invalidCapitalization: values.filter((value) => !isUnicodeUppercase(value)),
    },
  };
}

export function applyNamePreview(template: string, name: string): string {
  return template.replaceAll("{{name}}", name);
}

export function hasNamePlaceholder(template: string): boolean {
  return template.includes("{{name}}");
}
