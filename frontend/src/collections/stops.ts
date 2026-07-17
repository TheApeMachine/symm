import { createStore } from "@tanstack/react-store";

export type Stop = {
	symbol: string;
	stop_price: number;
	peak_return: number;
	stop_return: number;
	armed?: boolean;
	peak_price?: number;
	momentum?: number;
	peak_momentum?: number;
	momentum_floor?: number;
	momentum_health?: number;
	momentum_active?: boolean;
	peak_touch_count?: number;
	stagnation_max_touches?: number;
	stagnation_health?: number;
	stagnation_pending?: boolean;
	stagnation_active?: boolean;
};

type StopFrame = Record<string, unknown>;

const asFrame = (value: unknown, path: string): StopFrame => {
	if (typeof value === "object" && value !== null && !Array.isArray(value)) {
		return value as StopFrame;
	}

	throw new TypeError(`${path} must be an object`);
};

const requiredFinite = (value: unknown, path: string): number => {
	const number =
		typeof value === "number"
			? value
			: typeof value === "string" && value.trim().length > 0
				? Number(value)
				: Number.NaN;

	if (Number.isFinite(number)) {
		return number;
	}

	throw new TypeError(`${path} must be finite`);
};

const optionalFinite = (value: unknown): number | undefined => {
	const number =
		typeof value === "number"
			? value
			: typeof value === "string" && value.trim().length > 0
				? Number(value)
				: Number.NaN;

	return Number.isFinite(number) ? number : undefined;
};

const optionalBoolean = (value: unknown): boolean | undefined =>
	typeof value === "boolean" ? value : undefined;

const parseStop = (value: unknown, index: number): Stop => {
	const path = `stops[${index}]`;
	const frame = asFrame(value, path);

	if (typeof frame.symbol !== "string" || frame.symbol.length === 0) {
		throw new TypeError(`${path}.symbol must be a non-empty string`);
	}

	return {
		symbol: frame.symbol,
		stop_price: requiredFinite(frame.stop_price, `${path}.stop_price`),
		peak_return: requiredFinite(frame.peak_return, `${path}.peak_return`),
		stop_return: requiredFinite(frame.stop_return, `${path}.stop_return`),
		armed: optionalBoolean(frame.armed),
		peak_price: optionalFinite(frame.peak_price),
		momentum: optionalFinite(frame.momentum),
		peak_momentum: optionalFinite(frame.peak_momentum),
		momentum_floor: optionalFinite(frame.momentum_floor),
		momentum_health: optionalFinite(frame.momentum_health),
		momentum_active: optionalBoolean(frame.momentum_active),
		peak_touch_count: optionalFinite(frame.peak_touch_count),
		stagnation_max_touches: optionalFinite(frame.stagnation_max_touches),
		stagnation_health: optionalFinite(frame.stagnation_health),
		stagnation_pending: optionalBoolean(frame.stagnation_pending),
		stagnation_active: optionalBoolean(frame.stagnation_active),
	};
};

// The trader emits stops as a symbol-keyed map alongside the position frames.
export const normalizeStops = (stops: unknown): Record<string, Stop> => {
	const values = Array.isArray(stops)
		? stops
		: Object.values(asFrame(stops, "stops"));

	const parsed: Record<string, Stop> = {};

	for (const [index, value] of values.entries()) {
		const stop = parseStop(value, index);
		parsed[stop.symbol] = stop;
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
