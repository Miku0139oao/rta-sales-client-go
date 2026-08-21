export type DateRange = {
  from: string;
  to: string;
};

export type AlignedComparisonPeriods = {
  current: DateRange;
  previous: DateRange;
  previous2: DateRange;
  yearAgo: DateRange;
  yearAgoNext: DateRange;
};

const DAY_MS = 86_400_000;
const DAYS_PER_WEEK = 7;
const RETAIL_YEAR_DAYS = 52 * DAYS_PER_WEEK;
const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;

/**
 * Build weekday-aligned comparison windows for a custom sales range.
 *
 * The previous-period stride is the smallest whole number of weeks that can
 * contain the inclusive current range. Every current date therefore maps to
 * the same weekday, and the current/previous windows never overlap. For a
 * range of seven days or fewer, `previous` is the matching days last week and
 * `previous2` is the matching days two weeks ago.
 *
 * `yearAgo` uses the retail 52-week convention (364 days), rather than
 * subtracting a calendar year, so leap years cannot turn weekends into
 * weekdays. `yearAgoNext` retains the report's existing next-month baseline.
 * Invalid or inverted ISO date ranges return `undefined`.
 */
export function alignRangeComparisonPeriods(from: string, to: string): AlignedComparisonPeriods | undefined {
  const fromTimestamp = parseISODate(from);
  const toTimestamp = parseISODate(to);
  if (fromTimestamp === undefined || toTimestamp === undefined || toTimestamp < fromTimestamp) return undefined;

  const inclusiveDays = ((toTimestamp - fromTimestamp) / DAY_MS) + 1;
  const comparisonStride = Math.ceil(inclusiveDays / DAYS_PER_WEEK) * DAYS_PER_WEEK;
  const yearAgoNextMonth = shiftMonth(from.slice(0, 7), -11);
  return {
    current: { from, to },
    previous: shiftRange(fromTimestamp, toTimestamp, -comparisonStride),
    previous2: shiftRange(fromTimestamp, toTimestamp, -2 * comparisonStride),
    yearAgo: shiftRange(fromTimestamp, toTimestamp, -RETAIL_YEAR_DAYS),
    yearAgoNext: { from: `${yearAgoNextMonth}-01`, to: endOfMonth(yearAgoNextMonth) },
  };
}

function shiftRange(from: number, to: number, days: number): DateRange {
  const offset = days * DAY_MS;
  return { from: formatISODate(from + offset), to: formatISODate(to + offset) };
}

function parseISODate(value: string): number | undefined {
  if (!ISO_DATE.test(value)) return undefined;
  const [year, monthValue, day] = value.split('-').map(Number);
  const timestamp = Date.UTC(year!, monthValue! - 1, day!);
  return formatISODate(timestamp) === value ? timestamp : undefined;
}

function formatISODate(timestamp: number): string {
  const value = new Date(timestamp);
  return `${String(value.getUTCFullYear()).padStart(4, '0')}-${String(value.getUTCMonth() + 1).padStart(2, '0')}-${String(value.getUTCDate()).padStart(2, '0')}`;
}

function shiftMonth(value: string, months: number): string {
  const [year, monthValue] = value.split('-').map(Number);
  const date = new Date(Date.UTC(year!, monthValue! - 1 + months, 1));
  return `${date.getUTCFullYear()}-${String(date.getUTCMonth() + 1).padStart(2, '0')}`;
}

function endOfMonth(value: string): string {
  const [year, monthValue] = value.split('-').map(Number);
  return formatISODate(Date.UTC(year!, monthValue!, 0));
}
