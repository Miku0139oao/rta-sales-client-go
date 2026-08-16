import { describe, expect, it } from 'vitest';
import {
  defaultSalesReportFilter, includeInSalesReport, isCashCouponItem, isGiftCategoryItem, isStampItem,
} from './salesReportItems';
import type { SalesAnalysisItem } from './types';

function item(overrides: Partial<SalesAnalysisItem>): SalesAnalysisItem {
  return {
    storeId: '107', storeLabel: '107',
    category1: 'HEALTH', category1Code: 'A',
    category2: 'HEALTH CARE', category2Code: 'A01',
    category3: 'OTC', category3Code: 'A0101',
    category4: 'PAIN', category4Code: 'A010101',
    category5: 'OIL', category5Code: 'A01010101',
    articleCode: '100', articleName: 'Item', brandName: 'Brand',
    transactionCount: 1, saleQuantity: 1, saleAmount: 10,
    returnQuantity: 0, returnTransactionCount: 0, returnAmount: 0,
    netQuantity: 1, netSalesAmount: 10,
    ...overrides,
  };
}

describe('sales report item visibility', () => {
  it('keeps regular products and gift items that have sales', () => {
    expect(includeInSalesReport(item({ netSalesAmount: 10 }))).toBe(true);
    expect(includeInSalesReport(item({
      category2: 'HEALTH-FREE GIFT', category2Code: 'A04', articleName: '贈品面膜', netSalesAmount: 12,
    }))).toBe(true);
  });

  it('drops zero-amount gift items but keeps cash coupons', () => {
    const freeGift = item({
      category2: 'PC-FREE GIFT', category2Code: 'A07', articleName: '洗髮露贈品', netSalesAmount: 0, netQuantity: 80,
    });
    const giftBox = item({
      category2: 'GIFT', category2Code: 'B17', articleName: '節慶禮盒', netSalesAmount: 0, netQuantity: 12,
    });
    const coupon = item({
      category2: 'GIFT', category2Code: 'B17', articleName: 'Z11 萬寧現金優惠券$15', netSalesAmount: 0, netQuantity: 436,
    });
    expect(isGiftCategoryItem(freeGift)).toBe(true);
    expect(isGiftCategoryItem(giftBox)).toBe(true);
    expect(isCashCouponItem(coupon)).toBe(true);
    expect(includeInSalesReport(freeGift)).toBe(false);
    expect(includeInSalesReport(giftBox)).toBe(false);
    expect(includeInSalesReport(coupon)).toBe(true);
    expect(includeInSalesReport(item({
      category2: 'GIFT', category2Code: 'B17', articleName: '節慶禮盒', netSalesAmount: 0.01, netQuantity: 9,
    }))).toBe(false);
  });

  it('keeps a zero-amount item that is not in a gift category', () => {
    expect(includeInSalesReport(item({ articleName: '膠袋', netSalesAmount: 0, netQuantity: 669 }))).toBe(true);
  });

  it('drops stamp items even when they are booked at 0.01', () => {
    const stamp = item({ articleName: '缺貨-30 印花換購', netSalesAmount: 0.01, netQuantity: 82 });
    const stampGift = item({
      category2: 'GIFT', category2Code: 'B17', articleName: '印花換購禮', netSalesAmount: 0.01, netQuantity: 20,
    });
    expect(isStampItem(stamp)).toBe(true);
    expect(includeInSalesReport(stamp)).toBe(false);
    expect(includeInSalesReport(stampGift)).toBe(false);
  });

  it('uses a blacklist or whitelist of categories on top of the default rules', () => {
    const health = item({ category2: 'HEALTH CARE', category2Code: 'A01' });
    const paper = item({ category2: 'PAPER GOODS', category2Code: 'B12' });
    const gift = item({
      category2: 'GIFT', category2Code: 'B17', articleName: '節慶禮盒', netSalesAmount: 0,
    });
    expect(includeInSalesReport(health, { ...defaultSalesReportFilter(), categories: ['B12'] })).toBe(true);
    expect(includeInSalesReport(paper, { ...defaultSalesReportFilter(), categories: ['B12'] })).toBe(false);
    expect(includeInSalesReport(health, { ...defaultSalesReportFilter(), mode: 'whitelist', categories: ['A01'] })).toBe(true);
    expect(includeInSalesReport(paper, { ...defaultSalesReportFilter(), mode: 'whitelist', categories: ['A01'] })).toBe(false);
    expect(includeInSalesReport(gift, {
      mode: 'blacklist', excludeZeroGifts: false, excludeStamps: true, categories: [],
    })).toBe(true);
  });
});
