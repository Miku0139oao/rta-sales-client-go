import { render, cleanup, screen, fireEvent } from '@testing-library/svelte';
import { afterEach, it, expect, vi } from 'vitest';
import AnalysisDataTable from './AnalysisDataTable.svelte';
import { translator } from '../i18n';
import { filterAnalysisTable } from '../tableView';
import { workbookSnapshot, type AnalysisTable } from '../analysisTable';
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it('shows bounded mobile cards with every field available and can switch to the complete table', async () => {
  vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
  const table: AnalysisTable = {
    id: 'mobile',
    name: 'Mobile report',
    columns: [
      { label: 'Product', format: 'text' },
      { label: 'Sales', format: 'money' },
      { label: 'Units', format: 'number' },
      { label: 'Store', format: 'text' },
      { label: 'Code', format: 'text' },
    ],
    rows: Array.from({ length: 60 }, (_, index) => ({
      cells: [`Product ${index}`, index * 10, index, 'Central', `Code ${index}`],
      product: { column: 0, code: `${index}`, name: `Product ${index}` },
    })),
  };
  const onProduct = vi.fn();
  const onSort = vi.fn();
  render(AnalysisDataTable, { props: { table, t: translator('en'), locale: 'en', onSort, onProduct } });
  expect(screen.getAllByRole('article')).toHaveLength(50);
  await fireEvent.click(screen.getByRole('button', { name: 'View product details: Product 0' }));
  expect(onProduct).toHaveBeenCalledWith('0', 'Product 0');
  await fireEvent.change(screen.getByRole('combobox', { name: 'Sort by' }), { target: { value: '1' } });
  expect(onSort).toHaveBeenCalledWith('mobile', { column: 1, direction: 'descending' });
  await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
  expect(screen.getAllByRole('article')).toHaveLength(10);
  expect(screen.getByText('Code 59')).toBeInTheDocument();
  expect(workbookSnapshot([table], [], 'mobile.xlsx').sheets[0]?.rows).toHaveLength(60);
  await fireEvent.click(screen.getByRole('button', { name: 'Switch to full table' }));
  expect(screen.getAllByRole('row')).toHaveLength(61);
  expect(screen.getByText('Swipe horizontally to see all columns.')).toBeInTheDocument();
});
it('keeps 200k rows bounded in the DOM, exposes search and preserves complete export data', async () => {
  const table: AnalysisTable = {
    id: 'large',
    name: 'Large',
    columns: [{ label: 'SKU', format: 'text' }],
    rows: Array.from({ length: 200000 }, (_, i) => ({ cells: [String(i)] })),
  };
  const onSearch = vi.fn();
  render(AnalysisDataTable, {
    props: {
      table,
      t: translator('en'),
      locale: 'en',
      onSort: vi.fn(),
      onSearch,
    },
  });
  expect(screen.getAllByRole('row').length).toBeLessThan(30);
  await fireEvent.input(screen.getByRole('searchbox'), {
    target: { value: '199999' },
  });
  expect(onSearch).toHaveBeenCalledWith('large', '199999');
  const filtered = filterAnalysisTable(table, '199999');
  expect(workbookSnapshot([filtered], [], 'test.xlsx').sheets[0]?.rows).toEqual([['199999']]);
});
