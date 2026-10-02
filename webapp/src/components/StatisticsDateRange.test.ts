import { describe, it, expect, afterEach, vi } from 'vitest';

// timePeriods' getDateRange functions all read the module-level `todayDate`
// singleton, computed once from `new Date()` at import time — so testing
// them for a specific "today" means faking the system clock *before* the
// module is evaluated, via vi.resetModules() + a fresh dynamic import,
// rather than importing timePeriods statically at the top of this file.
async function timePeriodsAsOf(isoLocalDate: string) {
    vi.resetModules();
    vi.setSystemTime(new Date(isoLocalDate));
    const { timePeriods } = await import('./StatisticsDateRange');
    return timePeriods;
}

function rangeFor(periods: Awaited<ReturnType<typeof timePeriodsAsOf>>, value: string) {
    const period = periods.find((p) => p.value === value);
    if (!period) throw new Error(`no such period: ${value}`);
    return period.getDateRange();
}

describe('timePeriods', () => {
    afterEach(() => {
        vi.useRealTimers();
    });

    it('computes This/Last Week and This/Last Month for a mid-week date', async () => {
        // 2026-09-24 is a Thursday.
        const periods = await timePeriodsAsOf('2026-09-24T00:00:00');

        expect(rangeFor(periods, 'today')).toEqual({ startDate: '2026-09-24', endDate: '2026-09-24' });
        expect(rangeFor(periods, 'yesterday')).toEqual({ startDate: '2026-09-23', endDate: '2026-09-23' });
        expect(rangeFor(periods, 'thisWeek')).toEqual({ startDate: '2026-09-21', endDate: '2026-09-27' });
        expect(rangeFor(periods, 'lastWeek')).toEqual({ startDate: '2026-09-14', endDate: '2026-09-20' });
        expect(rangeFor(periods, 'thisMonth')).toEqual({ startDate: '2026-09-01', endDate: '2026-09-30' });
        expect(rangeFor(periods, 'lastMonth')).toEqual({ startDate: '2026-08-01', endDate: '2026-08-31' });
    });

    it('treats Sunday as the last day of the week, not the first', async () => {
        // 2026-09-20 is a Sunday — the edge case getDay()'s Sunday-is-0
        // convention could easily get backwards against this app's
        // Monday-start calendar weeks.
        const periods = await timePeriodsAsOf('2026-09-20T00:00:00');

        expect(rangeFor(periods, 'thisWeek')).toEqual({ startDate: '2026-09-14', endDate: '2026-09-20' });
        expect(rangeFor(periods, 'lastWeek')).toEqual({ startDate: '2026-09-07', endDate: '2026-09-13' });
    });

    it("rolls Last Month back into the previous year in January", async () => {
        const periods = await timePeriodsAsOf('2026-01-05T00:00:00');

        expect(rangeFor(periods, 'thisMonth')).toEqual({ startDate: '2026-01-01', endDate: '2026-01-31' });
        expect(rangeFor(periods, 'lastMonth')).toEqual({ startDate: '2025-12-01', endDate: '2025-12-31' });
    });

    it("This Week never drifts after being read more than once (regression: it used to mutate the shared today singleton)", async () => {
        const periods = await timePeriodsAsOf('2026-09-24T00:00:00');

        rangeFor(periods, 'thisWeek');
        rangeFor(periods, 'thisWeek');
        expect(rangeFor(periods, 'today')).toEqual({ startDate: '2026-09-24', endDate: '2026-09-24' });
    });
});
