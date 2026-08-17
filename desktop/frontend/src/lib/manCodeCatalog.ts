import { collectManCodes } from './manCodes';
import { AppError, type ManCodeGroup } from './types';

export const MAN_CODE_CATALOG_VERSION = 1;
export const MAN_CODE_CATALOG_MAX_BYTES = 1 << 20;

export function encodeManCodeCatalog(groups: ManCodeGroup[]): string {
  const document = {
    version: MAN_CODE_CATALOG_VERSION,
    groups: groups.map((group) => ({
      id: group.id,
      name: group.name,
      codes: [...group.codes],
    })),
  };
  const data = `${JSON.stringify(document, null, 2)}\n`;
  if (new TextEncoder().encode(data).length > MAN_CODE_CATALOG_MAX_BYTES) {
    throw new AppError('mancode_catalog_invalid', 'mancode catalog exceeds 1 MiB');
  }
  return data;
}

export function decodeManCodeCatalog(raw: string): ManCodeGroup[] {
  const bytes = new TextEncoder().encode(raw);
  if (bytes.length > MAN_CODE_CATALOG_MAX_BYTES) {
    throw new AppError('mancode_catalog_invalid', 'mancode catalog exceeds 1 MiB');
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new AppError('mancode_catalog_invalid', 'decode mancode catalog: unexpected end of JSON input');
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new AppError('mancode_catalog_invalid', 'decode mancode catalog: invalid');
  }
  const document = parsed as { version?: unknown; groups?: unknown };
  const extra = Object.keys(document).filter((key) => key !== 'version' && key !== 'groups');
  if (extra.length > 0) {
    throw new AppError('mancode_catalog_invalid', 'decode mancode catalog: unsupported fields');
  }
  if (document.version !== MAN_CODE_CATALOG_VERSION) {
    throw new AppError('mancode_catalog_version', `unsupported mancode catalog version ${String(document.version)}`);
  }
  if (!Array.isArray(document.groups)) {
    throw new AppError('mancode_catalog_invalid', 'mancode catalog groups must be an array');
  }
  const groups: ManCodeGroup[] = [];
  const ids = new Set<string>();
  const names = new Set<string>();
  for (const value of document.groups) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new AppError('mancode_catalog_invalid', 'decode mancode catalog: invalid');
    }
    const row = value as { id?: unknown; name?: unknown; codes?: unknown };
    const id = typeof row.id === 'string' ? row.id.trim() : '';
    const name = typeof row.name === 'string' ? row.name.trim() : '';
    if (!id || !name) throw new AppError('mancode_catalog_invalid', 'decode mancode catalog: invalid');
    if (ids.has(id) || names.has(name)) {
      throw new AppError('mancode_catalog_duplicate', 'duplicate mancode catalog group');
    }
    ids.add(id);
    names.add(name);
    const codes = Array.isArray(row.codes)
      ? collectManCodes(undefined, row.codes.filter((code): code is string => typeof code === 'string'))
      : [];
    groups.push({ id, name, codes });
  }
  return groups;
}
