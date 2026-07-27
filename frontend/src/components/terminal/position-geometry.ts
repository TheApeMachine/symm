import type { Holding, Stop } from "#/collections/types";

const clampPercent = (value: number, lo: number, hi: number): number => {
	if (!(hi > lo)) {
		return 50;
	}

	return Math.min(100, Math.max(0, ((value - lo) / (hi - lo)) * 100));
};

const numberValue = (value: number | string | undefined): number | null => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : null;
};

const positiveFinite = (value: number): number | null =>
	Number.isFinite(value) && value > 0 ? value : null;

/*
positionStop reads the nested stoploss published on the Holding/Position frame.
The legacy symbol-keyed stop map remains only as a fallback while old frames
still exist elsewhere on the wire.
*/
export const positionStop = (
	position: Holding,
	legacy?: Stop,
): Stop | undefined => {
	if (position.stoploss && typeof position.stoploss === "object") {
		return position.stoploss;
	}

	const stopPrice = position.stop_price;
	const stopReturn = position.stop_return;
	const peakReturn = position.peak_return;

	if (
		typeof stopPrice === "number" &&
		typeof stopReturn === "number" &&
		typeof peakReturn === "number" &&
		Number.isFinite(stopPrice) &&
		Number.isFinite(stopReturn) &&
		Number.isFinite(peakReturn)
	) {
		return {
			symbol: position.symbol,
			stop_price: stopPrice,
			stop_return: stopReturn,
			peak_return: peakReturn,
			armed:
				typeof position.stop_armed === "boolean"
					? position.stop_armed
					: undefined,
			momentum_active:
				typeof position.momentum_active === "boolean"
					? position.momentum_active
					: legacy?.momentum_active,
			momentum_health:
				typeof position.momentum_health === "number"
					? position.momentum_health
					: legacy?.momentum_health,
			stagnation_active:
				typeof position.stagnation_active === "boolean"
					? position.stagnation_active
					: legacy?.stagnation_active,
			stagnation_health:
				typeof position.stagnation_health === "number"
					? position.stagnation_health
					: legacy?.stagnation_health,
			stagnation_pending:
				typeof position.stagnation_pending === "boolean"
					? position.stagnation_pending
					: legacy?.stagnation_pending,
		};
	}

	return legacy;
};

export type PriceGaugeGeometry = {
	entryPct: number;
	markPct: number;
	stopPct: number | null;
	peakPct: number | null;
	rawMarkPrice: number | null;
};

/*
positionGaugeGeometry maps entry/mark/stop/peak onto a padded return domain.
*/
export const positionGaugeGeometry = (
	position: Holding,
	stop?: Stop,
): PriceGaugeGeometry | null => {
	const entry = numberValue(position.entry_price);
	const mark = numberValue(position.mark);
	const returnPct = numberValue(position.return_pct);

	if (entry === null || entry <= 0) {
		return null;
	}

	const rawMark = mark === null ? null : positiveFinite(mark);
	const derivedMark =
		rawMark ??
		(returnPct !== null && returnPct > -1
			? positiveFinite(entry * (1 + returnPct))
			: null);
	const markReturn = derivedMark === null ? 0 : derivedMark / entry - 1;
	const armed =
		stop !== undefined &&
		stop.armed !== false &&
		positiveFinite(stop.stop_price) !== null &&
		Number.isFinite(stop.stop_return) &&
		Number.isFinite(stop.peak_return);

	// Unarmed / missing stops: show entry↔mark only. Do not pretend the mark
	// extreme is take-profit — that was reading as "stop at TP".
	if (!armed || stop === undefined) {
		const lo = Math.min(0, markReturn);
		const hi = Math.max(0, markReturn);

		if (!(hi > lo)) {
			return null;
		}

		const pad = Math.max((hi - lo) * 0.15, 1e-6);
		const domainLo = lo - pad;
		const domainHi = hi + pad;

		return {
			entryPct: clampPercent(0, domainLo, domainHi),
			markPct: clampPercent(markReturn, domainLo, domainHi),
			stopPct: null,
			peakPct: null,
			rawMarkPrice: rawMark,
		};
	}

	const stopReturn = stop.stop_return;
	const peakReturn = stop.peak_return;
	const trailSpan = peakReturn - stopReturn;
	const rawLo = Math.min(0, stopReturn, markReturn);
	const rawHi = Math.max(0, peakReturn, markReturn);
	// Keep stop and peak from collapsing onto one pixel when the trail is thin.
	const padding = Math.max(trailSpan, rawHi - rawLo, 1e-6) * 0.5;

	if (!(padding > 0)) {
		return null;
	}

	const lo = rawLo - padding;
	const hi = rawHi + padding;

	return {
		entryPct: clampPercent(0, lo, hi),
		markPct: clampPercent(markReturn, lo, hi),
		stopPct: clampPercent(stopReturn, lo, hi),
		peakPct: clampPercent(peakReturn, lo, hi),
		rawMarkPrice: rawMark,
	};
};
