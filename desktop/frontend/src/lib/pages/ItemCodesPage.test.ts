import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { configureBackend, setLatestArticleNames } from '../backend';
import { translator } from '../i18n';
import type { ManCodeCatalogTransferResult, ManCodeGroup, SaveManCodeGroupRequest } from '../types';
import ItemCodesPage from './ItemCodesPage.svelte';

const health: ManCodeGroup = { id: 'group-health', name: '保健', codes: ['123456', '234567'] };
const skin: ManCodeGroup = { id: 'group-skin', name: '護膚', codes: ['552646'] };

function button(container: HTMLElement, label: string): Element {
  const found = [...container.querySelectorAll('md-filled-button, md-filled-tonal-button, md-outlined-button, md-text-button')]
    .find((element) => element.textContent?.includes(label));
  if (!found) throw new Error(`Button not found: ${label}`);
  return found;
}

afterEach(() => {
  cleanup();
  configureBackend(undefined);
});

describe('item code management', () => {
  it('uses a tag icon rather than a QR or barcode for the empty catalog', async () => {
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => []),
    } });
    const { container } = render(ItemCodesPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });

    await waitFor(() => expect(screen.getByText('尚未建立組別')).toBeInTheDocument());
    expect(container.querySelector('.empty-state .material-symbols-rounded')).toHaveTextContent('tag');
    expect(container.querySelector('.empty-state .material-symbols-rounded')).not.toHaveTextContent('qr_code_2');
    expect(container.querySelector('.empty-state .material-symbols-rounded')).not.toHaveTextContent('barcode');
    expect(container).not.toHaveTextContent('qr_code_2');
    expect(container).not.toHaveTextContent('barcode');
  });

  it('lists groups, expands codes, and shows article names from the latest analysis', async () => {
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => [health, skin]),
    } });
    setLatestArticleNames({ '552646': 'AHC 安瓶精華纖維面膜' });
    const { container } = render(ItemCodesPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });

    await waitFor(() => expect(screen.getByText('保健')).toBeInTheDocument());
    expect(screen.getByText('2 個代碼')).toBeInTheDocument();
    expect(screen.getByText('護膚')).toBeInTheDocument();
    expect(screen.queryByText('552646')).not.toBeInTheDocument();

    await fireEvent.click(container.querySelector('[aria-label="展開 護膚"]')!);
    await waitFor(() => expect(screen.getByText('552646')).toBeInTheDocument());
    expect(screen.getByText('AHC 安瓶精華纖維面膜')).toBeInTheDocument();
  });

  it('shows codes only when no analysis names are available', async () => {
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => [skin]),
    } });
    const { container } = render(ItemCodesPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('護膚')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('[aria-label="展開 護膚"]')!);
    await waitFor(() => expect(screen.getByText('552646')).toBeInTheDocument());
    expect(screen.queryByText('AHC 安瓶精華纖維面膜')).not.toBeInTheDocument();
  });

  it('adds, renames, and confirms deleting a group', async () => {
    let store: ManCodeGroup[] = [health];
    const save = vi.fn(async (input: unknown) => {
      const request = input as SaveManCodeGroupRequest;
      if (request.id) {
        const updated = { ...store.find((group) => group.id === request.id)!, name: request.name };
        store = store.map((group) => group.id === updated.id ? updated : group);
        return updated;
      }
      const created = { id: 'group-new', name: request.name, codes: [] };
      store = [...store, created];
      return created;
    });
    const remove = vi.fn(async () => { store = store.filter((group) => group.id !== health.id); });
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => store),
      SaveManCodeGroup: save,
      DeleteManCodeGroup: remove,
    } });
    const { container } = render(ItemCodesPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('保健')).toBeInTheDocument());

    await fireEvent.click(button(container, '新增組別'));
    await fireEvent.input(screen.getByLabelText('組別名稱'), { target: { value: '個護' } });
    await fireEvent.click(button(container, '儲存'));
    await waitFor(() => expect(screen.getByText('個護')).toBeInTheDocument());
    expect(save).toHaveBeenCalledWith({ name: '個護' });

    await fireEvent.click(container.querySelector('[aria-label="重新命名 保健"]')!);
    await fireEvent.input(screen.getByLabelText('組別名稱'), { target: { value: '保健品' } });
    await fireEvent.click(button(container, '儲存'));
    await waitFor(() => expect(screen.getByText('保健品')).toBeInTheDocument());
    expect(save).toHaveBeenLastCalledWith({ id: health.id, name: '保健品' });

    await fireEvent.click(container.querySelector('[aria-label="刪除 保健品"]')!);
    expect(screen.getByText('刪除組別？')).toBeInTheDocument();
    await fireEvent.click(button(container, '刪除'));
    await waitFor(() => expect(screen.queryByText('保健品')).not.toBeInTheDocument());
    expect(remove).toHaveBeenCalledWith(health.id);
  });

  it('pastes a batch of codes, skips duplicates, and deletes a single code', async () => {
    let group: ManCodeGroup = { ...health };
    const replace = vi.fn(async (input: unknown) => {
      const request = input as { id: string; codes?: string[] };
      group = { ...group, codes: request.codes ?? [] };
      return group;
    });
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => [group]),
      ReplaceManCodeGroupCodes: replace,
    } });
    const { container } = render(ItemCodesPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('保健')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('[aria-label="展開 保健"]')!);
    await waitFor(() => expect(screen.getByLabelText('貼上商品代碼')).toBeInTheDocument());

    await fireEvent.input(screen.getByLabelText('貼上商品代碼'), { target: { value: '123456, 999888，888777 999888' } });
    await fireEvent.click(button(container, '加入代碼'));
    await waitFor(() => expect(replace).toHaveBeenCalledWith({
      id: health.id, codes: ['123456', '234567', '999888', '888777'],
    }));
    expect(screen.getByText('已略過 2 個重複代碼。')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('999888')).toBeInTheDocument());

    await fireEvent.click(container.querySelector('[aria-label="刪除 234567"]')!);
    await waitFor(() => expect(replace).toHaveBeenLastCalledWith({
      id: health.id, codes: ['123456', '999888', '888777'],
    }));
    expect(screen.queryByText('234567')).not.toBeInTheDocument();
  });

  it('filters groups by name or code', async () => {
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => [health, skin]),
    } });
    const { container } = render(ItemCodesPage, { props: { t: translator('zh-TW'), locale: 'zh-TW' } });
    await waitFor(() => expect(screen.getByText('護膚')).toBeInTheDocument());

    await fireEvent.input(screen.getByLabelText('搜尋組別或代碼'), { target: { value: '護膚' } });
    expect(screen.getByText('護膚')).toBeInTheDocument();
    expect(screen.queryByText('保健')).not.toBeInTheDocument();

    await fireEvent.input(screen.getByLabelText('搜尋組別或代碼'), { target: { value: '123456' } });
    expect(screen.getByText('保健')).toBeInTheDocument();
    expect(screen.getByText('123456')).toBeInTheDocument();
    expect(screen.queryByText('護膚')).not.toBeInTheDocument();

    await fireEvent.click(container.querySelector('[aria-label="收起 保健"]')!);
    await waitFor(() => expect(screen.queryByText('123456')).not.toBeInTheDocument());
  });

  it('keeps the paste draft when adding codes fails', async () => {
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => [health]),
      ReplaceManCodeGroupCodes: vi.fn(async () => { throw new Error('failed'); }),
    } });
    const { container } = render(ItemCodesPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('保健')).toBeInTheDocument());
    await fireEvent.click(container.querySelector('[aria-label="展開 保健"]')!);
    await waitFor(() => expect(screen.getByLabelText('貼上商品代碼')).toBeInTheDocument());
    await fireEvent.input(screen.getByLabelText('貼上商品代碼'), { target: { value: '999888' } });
    await fireEvent.click(button(container, '加入代碼'));

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    expect((screen.getByLabelText('貼上商品代碼') as HTMLTextAreaElement).value).toBe('999888');
  });

  it('exports the catalog and ignores a cancelled save dialog', async () => {
    const exported = vi.fn(async (): Promise<ManCodeCatalogTransferResult> => (
      { cancelled: false, path: 'D:\\item-codes.json', groups: [health] }
    ));
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => [health]),
      ExportManCodeCatalog: exported,
    } });
    const { container } = render(ItemCodesPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('保健')).toBeInTheDocument());
    expect(button(container, '匯出').querySelector('.material-symbols-rounded')).toHaveTextContent('upload');
    expect(button(container, '匯入').querySelector('.material-symbols-rounded')).toHaveTextContent('download');

    await fireEvent.click(button(container, '匯出'));
    await waitFor(() => expect(screen.getByText('已匯出商品代碼目錄。')).toBeInTheDocument());
    expect(exported).toHaveBeenCalledTimes(1);
    expect(screen.getByText('保健')).toBeInTheDocument();

    exported.mockResolvedValueOnce({ cancelled: true });
    await fireEvent.click(button(container, '匯出'));
    await waitFor(() => expect(exported).toHaveBeenCalledTimes(2));
    expect(screen.getByText('已匯出商品代碼目錄。')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByText('保健')).toBeInTheDocument();
  });

  it('imports a catalog, refreshes the list, and leaves the page unchanged when cancelled or invalid', async () => {
    const incoming: ManCodeGroup[] = [{ id: 'group-care', name: '個護', codes: ['888777'] }];
    const imported = vi.fn(async (): Promise<ManCodeCatalogTransferResult> => (
      { cancelled: false, path: 'D:\\item-codes.json', groups: incoming }
    ));
    configureBackend({ methods: {
      ListManCodeGroups: vi.fn(async () => [health, skin]),
      ImportManCodeCatalog: imported,
    } });
    const { container } = render(ItemCodesPage, {
      props: { t: translator('zh-TW'), locale: 'zh-TW' },
    });
    await waitFor(() => expect(screen.getByText('保健')).toBeInTheDocument());

    await fireEvent.click(button(container, '匯入'));
    await waitFor(() => expect(screen.getByText('個護')).toBeInTheDocument());
    expect(screen.getByText('已匯入 1 個組別。')).toBeInTheDocument();
    expect(screen.queryByText('保健')).not.toBeInTheDocument();
    expect(screen.queryByText('護膚')).not.toBeInTheDocument();

    imported.mockResolvedValueOnce({ cancelled: true });
    await fireEvent.click(button(container, '匯入'));
    await waitFor(() => expect(imported).toHaveBeenCalledTimes(2));
    expect(screen.getByText('個護')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    imported.mockRejectedValueOnce(new Error('decode mancode catalog: unexpected end of JSON'));
    await fireEvent.click(button(container, '匯入'));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('商品代碼目錄格式不正確，現有資料未變更。'));
    expect(screen.getByText('個護')).toBeInTheDocument();
    expect(screen.queryByText('保健')).not.toBeInTheDocument();
  });
});
