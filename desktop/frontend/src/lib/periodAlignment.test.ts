import { describe, expect, it } from 'vitest';
import { alignRangeComparisonPeriods } from './periodAlignment';

function weekdays(from: string, to: string): number[] {
  const start = new Date(`${from}T00:00:00Z`);
  const end = new Date(`${to}T00:00:00Z`);
  const days: number[] = [];
  for (const day = new Date(start); day <= end; day.setUTCDate(day.getUTCDate() + 1)) {
    days.push(day.getUTCDay());
  }
  return days;
}

function shiftCalendarDays(from: string, to: string, days: number): { from: string; to: string } {
  const add = (value: string, offset: number) => {
    const date = new Date(`${value}T00:00:00Z`);
    date.setUTCDate(date.getUTCDate() + offset);
    return date.toISOString().slice(0, 10);
  };
  return { from: add(from, days), to: add(to, days) };
}

describe('weekday-aligned range comparison periods', () => {
  it('maps Fri-Sun onto the previous Fri-Sun, including last weekend', () => {
    const periods = alignRangeComparisonPeriods('2026-08-07', '2026-08-09');
    expect(periods).toBeDefined();
    expect(weekdays('2026-08-07', '2026-08-09')).toEqual([5, 6, 0]);
    expect(periods!.current).toEqual({ from: '2026-08-07', to: '2026-08-09' });
    expect(periods!.previous).toEqual({ from: '2026-07-31', to: '2026-08-02' });
    expect(weekdays(periods!.previous.from, periods!.previous.to)).toEqual([5, 6, 0]);
    expect(periods!.previous).not.toEqual(shiftCalendarDays('2026-08-07', '2026-08-09', -3));
    expect(periods!.previous2).toEqual({ from: '2026-07-24', to: '2026-07-26' });
    expect(weekdays(periods!.previous2.from, periods!.previous2.to)).toEqual([5, 6, 0]);
    expect(periods!.yearAgo).toEqual({ from: '2025-08-08', to: '2025-08-10' });
    expect(weekdays(periods!.yearAgo.from, periods!.yearAgo.to)).toEqual([5, 6, 0]);
    expect(periods!.yearAgoNext).toEqual({ from: '2025-09-01', to: '2025-09-30' });
  });

  it('maps Mon-Wed onto last week Mon-Wed instead of the preceding weekend', () => {
    const periods = alignRangeComparisonPeriods('2026-08-03', '2026-08-05');
    expect(periods).toBeDefined();
    expect(weekdays('2026-08-03', '2026-08-05')).toEqual([1, 2, 3]);
    expect(periods!.previous).toEqual({ from: '2026-07-27', to: '2026-07-29' });
    expect(weekdays(periods!.previous.from, periods!.previous.to)).toEqual([1, 2, 3]);
    expect(periods!.previous).not.toEqual({ from: '2026-07-31', to: '2026-08-02' });
    expect(weekdays('2026-07-31', '2026-08-02')).toEqual([5, 6, 0]);
    expect(periods!.previous2).toEqual({ from: '2026-07-20', to: '2026-07-22' });
    expect(periods!.yearAgo).toEqual({ from: '2025-08-04', to: '2025-08-06' });
    expect(weekdays(periods!.yearAgo.from, periods!.yearAgo.to)).toEqual([1, 2, 3]);
    expect(periods!.yearAgoNext).toEqual({ from: '2025-09-01', to: '2025-09-30' });
  });

  it('keeps a mixed Sat-Mon range on the previous matching weekend plus Monday', () => {
    const periods = alignRangeComparisonPeriods('2026-08-01', '2026-08-03');
    expect(periods).toBeDefined();
    expect(weekdays('2026-08-01', '2026-08-03')).toEqual([6, 0, 1]);
    expect(periods!.previous).toEqual({ from: '2026-07-25', to: '2026-07-27' });
    expect(weekdays(periods!.previous.from, periods!.previous.to)).toEqual([6, 0, 1]);
    expect(periods!.previous).not.toEqual(shiftCalendarDays('2026-08-01', '2026-08-03', -3));
    expect(weekdays('2026-07-29', '2026-07-31')).toEqual([3, 4, 5]);
    expect(periods!.previous2).toEqual({ from: '2026-07-18', to: '2026-07-20' });
    expect(weekdays(periods!.yearAgo.from, periods!.yearAgo.to)).toEqual([6, 0, 1]);
    expect(periods!.yearAgo).toEqual({ from: '2025-08-02', to: '2025-08-04' });
    expect(periods!.yearAgoNext).toEqual({ from: '2025-09-01', to: '2025-09-30' });
  });

  it('maps a 1-3 weekend (Fri-Sun) onto the previous weekend, not the weekdays immediately before', () => {
    const periods = alignRangeComparisonPeriods('2027-01-01', '2027-01-03');
    expect(periods).toBeDefined();
    expect(weekdays('2027-01-01', '2027-01-03')).toEqual([5, 6, 0]);
    expect(periods!.previous).toEqual({ from: '2026-12-25', to: '2026-12-27' });
    expect(weekdays(periods!.previous.from, periods!.previous.to)).toEqual([5, 6, 0]);
    expect(periods!.previous).not.toEqual({ from: '2026-12-29', to: '2026-12-31' });
    expect(weekdays('2026-12-29', '2026-12-31')).toEqual([2, 3, 4]);
    expect(periods!.previous2).toEqual({ from: '2026-12-18', to: '2026-12-20' });
    expect(weekdays(periods!.yearAgo.from, periods!.yearAgo.to)).toEqual([5, 6, 0]);
  });

  it('maps a full Monday-Sunday week onto the two preceding full weeks', () => {
    const periods = alignRangeComparisonPeriods('2026-08-03', '2026-08-09');
    expect(periods).toBeDefined();
    expect(periods!.previous).toEqual({ from: '2026-07-27', to: '2026-08-02' });
    expect(periods!.previous2).toEqual({ from: '2026-07-20', to: '2026-07-26' });
    expect(periods!.yearAgo).toEqual({ from: '2025-08-04', to: '2025-08-10' });
    expect(weekdays(periods!.current.from, periods!.current.to)).toEqual(weekdays(periods!.previous.from, periods!.previous.to));
  });

  it('rounds a multi-week range up to a non-overlapping whole-week stride', () => {
    const periods = alignRangeComparisonPeriods('2026-08-01', '2026-08-10');
    expect(periods).toBeDefined();
    expect(periods!.previous).toEqual({ from: '2026-07-18', to: '2026-07-27' });
    expect(periods!.previous2).toEqual({ from: '2026-07-04', to: '2026-07-13' });
    expect(periods!.yearAgo).toEqual({ from: '2025-08-02', to: '2025-08-11' });
  });

  it('keeps the year-ago weekdays stable across leap day', () => {
    const periods = alignRangeComparisonPeriods('2025-02-28', '2025-03-02');
    expect(periods).toBeDefined();
    expect(periods!.yearAgo).toEqual({ from: '2024-03-01', to: '2024-03-03' });
    expect(weekdays(periods!.yearAgo.from, periods!.yearAgo.to)).toEqual(weekdays(periods!.current.from, periods!.current.to));
  });

  it('rejects inverted or malformed ranges', () => {
    expect(alignRangeComparisonPeriods('2026-08-03', '2026-08-01')).toBeUndefined();
    expect(alignRangeComparisonPeriods('', '2026-08-01')).toBeUndefined();
    expect(alignRangeComparisonPeriods('08-01', '08-03')).toBeUndefined();
    expect(alignRangeComparisonPeriods('2026-02-30', '2026-03-01')).toBeUndefined();
  });
});
