import { createStore } from "@tanstack/react-store";

export type Stop = {
	symbol: string;
	stop_price: number;
	peak_return: number;
	stop_return: number;
	momentum: number;
	peak_momentum: number;
	momentum_floor: number;
	momentum_health: number;
	momentum_active: boolean;
	peak_touch_count: number;
	stagnation_max_touches: number;
	stagnation_health: number;
	stagnation_pending: boolean;
	stagnation_active: boolean;
};

type StopFrame = Record<string, unknown>;

const asFrame = (value: unknown): StopFrame | null =>
	typeof value === "object" && value !== null && !Array.isArray(value)
		? (value as StopFrame)
		: null;

const finite = (value: unknown): number | null => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : null;
};

const parseStop = (value: unknown): Stop | null => {
	const frame = asFrame(value);

	if (frame === null || typeof frame.symbol !== "string") {
		return null;
	}

	const stopPrice = finite(frame.stop_price);
	const peakReturn = finite(frame.peak_return);
	const stopReturn = finite(frame.stop_return);

	if (stopPrice === null || peakReturn === null || stopReturn === null) {
		return null;
	}

	return {
		symbol: frame.symbol,
		stop_price: stopPrice,
		peak_return: peakReturn,
		stop_return: stopReturn,
		momentum: finite(frame.momentum) ?? 0,
		peak_momentum: finite(frame.peak_momentum) ?? 0,
		momentum_floor: finite(frame.momentum_floor) ?? 0,
		momentum_health: finite(frame.momentum_health) ?? 0,
		momentum_active: frame.momentum_active === true,
		peak_touch_count: typeof frame.peak_touch_count === "number" ? frame.peak_touch_count : 0,
		stagnation_max_touches: typeof frame.stagnation_max_touches === "number" ? frame.stagnation_max_touches : 0,
		stagnation_health: finite(frame.stagnation_health) ?? 1,
		stagnation_pending: frame.stagnation_pending === true,
		stagnation_active: frame.stagnation_active === true,
	};
};

// The trader emits stops as a symbol-keyed map alongside the position frames.
export const normalizeStops = (stops: unknown): Record<string, Stop> => {
	const frame = asFrame(stops);

	if (frame === null) {
		return {};
	}

	const parsed: Record<string, Stop> = {};

	for (const value of Object.values(frame)) {
		const stop = parseStop(value);

		if (stop !== null) {
			parsed[stop.symbol] = stop;
		}
	}

	return parsed;
};

export const stopsStore = createStore(
	{
		stops: {} as Record<string, Stop>,
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (stops: unknown) =>
			setState(() => ({
				stops: normalizeStops(stops),
				observed: true,
			})),
	}),
);