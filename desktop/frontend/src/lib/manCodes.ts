export function splitManCodes(raw: string): string[] {
  return raw.split(/[\s,，]+/u).map((part) => part.trim()).filter(Boolean);
}

export function uniqueManCodes(values: string[]): { unique: string[]; duplicateCount: number } {
  const seen = new Set<string>();
  const unique: string[] = [];
  let duplicateCount = 0;
  for (const value of values) {
    if (seen.has(value)) {
      duplicateCount += 1;
      continue;
    }
    seen.add(value);
    unique.push(value);
  }
  return { unique, duplicateCount };
}

export function collectManCodes(raw?: string, codes?: string[]): string[] {
  return uniqueManCodes([...(codes ?? []), ...splitManCodes(raw ?? '')]).unique;
}
