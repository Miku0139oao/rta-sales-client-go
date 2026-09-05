import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { translator } from '../i18n';
import type { SalesInsight, SalesInsights } from '../salesInsights';
import AnalysisInsights from './AnalysisInsights.svelte';

afterEach(() => cleanup());

const t = translator('zh-TW');
const longName = '超長名稱保濕修護精華液與日常肌膚護理 Super-Long Hydrating Repair Essence Daily Skin Care 50ml 限量組合';

function period(label: string, from: string, to: string): NonNullable<SalesInsights['current']> {
  return {
    key: label, label, from, to, complete: true, successfulStores: 1,
    totals: { saleQuantity: 0, saleAmount: 0, returnQuantity: 0, returnAmount: 0, netQuantity: 0, netSalesAmount: 0 },
    stores: [],
  };
}

function entry(kind: SalesInsight['kind'], code: string, name: string, extra: Partial<SalesInsight> = {}): SalesInsight {
  return {
    kind, code, name, current: 1300, previous: 650, difference: 650, percent: 1, quantity: 10, refunds: 0, ...extra,
  };
}

function renderInsights(data: Pick<SalesInsights, 'entries' | 'reason'> & Partial<SalesInsights>, onProduct = vi.fn()) {
  return {
    onProduct,
    ...render(AnalysisInsights, {
      props: {
        t, locale: 'zh-TW',
        data: {
          current: period('本期', '2026-08-01', '2026-08-31'),
          previous: period('上期', '2026-07-01', '2026-07-31'),
          ...data,
        },
        onProduct,
      },
    }),
  };
}

describe('analysis insight card presentation', () => {
  it('shows the empty note without cards when the range is ready and has no highlights', () => {
    renderInsights({ entries: [], reason: 'ready' });
    const region = screen.getByRole('region', { name: '分析重點' });
    expect(within(region).getByText('此範圍目前沒有可列出的銷售重點。')).toBeInTheDocument();
    expect(region.querySelectorAll('article')).toHaveLength(0);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('surfaces a missing-data note and no ranking cards', () => {
    renderInsights({ entries: [], reason: 'currentMissing' });
    expect(screen.getByRole('status')).toHaveTextContent('本期明細尚未齊全，暫不產生分析重點。');
    expect(screen.queryByText('此範圍目前沒有可列出的銷售重點。')).not.toBeInTheDocument();
    expect(screen.getByRole('region', { name: '分析重點' }).querySelectorAll('article')).toHaveLength(0);
  });

  it('renders one accessible product button for a single highlight', async () => {
    const { onProduct } = renderInsights({
      entries: [entry('growth', '00100', '原生成商品 01')],
      reason: 'ready',
    });
    const region = screen.getByRole('region', { name: '分析重點' });
    expect(region.querySelectorAll('article.insight-card')).toHaveLength(1);
    expect(within(region).getByRole('heading', { name: '淨銷售成長最多' })).toBeInTheDocument();
    const trigger = within(region).getByRole('button', { name: '追查分析重點：原生成商品 01' });
    expect(trigger).toHaveAttribute('type', 'button');
    expect(trigger).toHaveTextContent('00100');
    trigger.focus();
    expect(trigger).toHaveFocus();
    await fireEvent.click(trigger);
    expect(onProduct).toHaveBeenCalledTimes(1);
    expect(onProduct).toHaveBeenCalledWith('00100', '原生成商品 01');
  });

  it('keeps three distinct ranking cards including a decline treatment', () => {
    renderInsights({
      entries: [
        entry('decline', '00002', '下滑商品', { current: 20, previous: 80, difference: -60, percent: -0.75 }),
        entry('growth', '00107', '成長商品', { current: 90, previous: 30, difference: 60, percent: 2 }),
        entry('returns', '00999', '退款商品', { refunds: 15, current: 40, previous: 40, difference: 0, percent: 0 }),
      ],
      reason: 'ready',
    });
    const region = screen.getByRole('region', { name: '分析重點' });
    const cards = region.querySelectorAll('article');
    expect(cards).toHaveLength(3);
    expect(cards[0]).toHaveClass('insight-card', 'decline');
    expect(cards[1]).toHaveClass('insight-card');
    expect(within(region).getByRole('heading', { name: '淨銷售下滑最多' })).toBeInTheDocument();
    expect(within(region).getByRole('heading', { name: '淨銷售成長最多' })).toBeInTheDocument();
    expect(within(region).getByRole('heading', { name: '退款金額最高' })).toBeInTheDocument();
    expect(within(region).getAllByRole('button', { name: /^追查分析重點/ })).toHaveLength(3);
  });

  it('wraps long product names on the inspect control', () => {
    renderInsights({
      entries: [entry('growth', '00100', longName)],
      reason: 'ready',
    });
    const trigger = screen.getByRole('button', { name: `追查分析重點：${longName}` });
    expect(trigger).toHaveTextContent(longName);
    expect(getComputedStyle(trigger).overflowWrap).toBe('anywhere');
  });

  it('leaves calculation-basis details in place for the existing disclosure', () => {
    renderInsights({ entries: [entry('leader', '00100', '主力商品', { previous: null, difference: null, percent: null })], reason: 'ready' });
    const details = screen.getByText('計算依據與範圍').closest('details');
    expect(details).not.toBeNull();
    expect(details).toHaveTextContent('本期 · 2026-08-01 — 2026-08-31');
    expect(details).toHaveTextContent('成長與下滑以完整、相同門店範圍的期間總額差排序');
  });
});
