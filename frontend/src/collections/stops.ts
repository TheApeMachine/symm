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

const optionalFinite = (value: unknown, path: string): number | undefined => {
	if (value === undefined) {
		return undefined;
	}

	const number =
		typeof value === "number"
			? value
			: typeof value === "string" && value.trim().length > 0
				? Number(value)
				: Number.NaN;

	if (Number.isFinite(number)) {
		return number;
	}

	throw new TypeError(`${path} must be finite when present`);
};

const optionalBoolean = (value: unknown, path: string): boolean | undefined => {
	if (value === undefined) {
		return undefined;
	}

	if (typeof value === "boolean") {
		return value;
	}

	throw new TypeError(`${path} must be a boolean when present`);
};

const parseStop = (value: unknown, index: number): Stop => {
	const path = `stops[${index}]`;
	const frame = asFrame(value, path);

	if (typeof frame.symbol !== "string" || frame.symbol.length === 0) {
		throw new TypeError(`${path}.symbol must be a non-empty string`);
	}

	const momentumActive = optionalBoolean(
		frame.momentum_active,
		`${path}.momentum_active`,
	);
	const stagnationActive = optionalBoolean(
		frame.stagnation_active,
		`${path}.stagnation_active`,
	);
	const momentumHealth = optionalFinite(
		frame.momentum_health,
		`${path}.momentum_health`,
	);
	const stagnationHealth = optionalFinite(
		frame.stagnation_health,
		`${path}.stagnation_health`,
	);

	if (momentumActive === true && momentumHealth === undefined) {
		throw new TypeError(
			`${path}.momentum_health is required when momentum_active is true`,
		);
	}

	if (stagnationActive === true && stagnationHealth === undefined) {
		throw new TypeError(
			`${path}.stagnation_health is required when stagnation_active is true`,
		);
	}

	return {
		symbol: frame.symbol,
		stop_price: requiredFinite(frame.stop_price, `${path}.stop_price`),
		peak_return: requiredFinite(frame.peak_return, `${path}.peak_return`),
		stop_return: requiredFinite(frame.stop_return, `${path}.stop_return`),
		armed: optionalBoolean(frame.armed, `${path}.armed`),
		peak_price: optionalFinite(frame.peak_price, `${path}.peak_price`),
		momentum: optionalFinite(frame.momentum, `${path}.momentum`),
		peak_momentum: optionalFinite(frame.peak_momentum, `${path}.peak_momentum`),
		momentum_floor: optionalFinite(
			frame.momentum_floor,
			`${path}.momentum_floor`,
		),
		momentum_health: momentumHealth,
		momentum_active: momentumActive,
		peak_touch_count: optionalFinite(
			frame.peak_touch_count,
			`${path}.peak_touch_count`,
		),
		stagnation_max_touches: optionalFinite(
			frame.stagnation_max_touches,
			`${path}.stagnation_max_touches`,
		),
		stagnation_health: stagnationHealth,
		stagnation_pending: optionalBoolean(
			frame.stagnation_pending,
			`${path}.stagnation_pending`,
		),
		stagnation_active: stagnationActive,
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
