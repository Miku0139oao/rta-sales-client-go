import { describe, expect, it } from 'vitest';
import { decodeManCodeCatalog, encodeManCodeCatalog } from './manCodeCatalog';

describe('item code catalog transfer', () => {
  it('round-trips a versioned catalog', () => {
    const groups = [{ id: 'g1', name: '保健', codes: ['123456', '234567'] }];
    const encoded = encodeManCodeCatalog(groups);
    expect(encoded).toContain('"version": 1');
    expect(decodeManCodeCatalog(encoded)).toEqual(groups);
  });

  it('rejects an unsupported version', () => {
    expect(() => decodeManCodeCatalog(JSON.stringify({ version: 9, groups: [] }))).toThrow(
      expect.objectContaining({ code: 'mancode_catalog_version' }),
    );
  });

  it('rejects a missing groups array', () => {
    expect(() => decodeManCodeCatalog(JSON.stringify({ version: 1 }))).toThrow(
      expect.objectContaining({ code: 'mancode_catalog_invalid' }),
    );
  });

  it('rejects duplicate group names', () => {
    expect(() => decodeManCodeCatalog(JSON.stringify({
      version: 1,
      groups: [
        { id: 'a', name: '保健', codes: [] },
        { id: 'b', name: '保健', codes: [] },
      ],
    }))).toThrow(expect.objectContaining({ code: 'mancode_catalog_duplicate' }));
  });
});
