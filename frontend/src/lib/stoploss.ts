import type {
	DynamicStoploss,
	DynamicStoplossState,
	StoplossStateCode,
} from "#/collections/types";

// Exact match for broker.StoplossState iota. Wire 2 is legacy TakeProfit → Armed.
const STATE_FROM_CODE: Record<StoplossStateCode, DynamicStoplossState> = {
	0: "Healthy",
	1: "Armed",
	2: "Armed",
	3: "Triggered",
};

const STATE_LABELS = new Set<string>(["Healthy", "Armed", "Triggered"]);

const num = (value: unknown): number | null => {
	if (typeof value === "number" && Number.isFinite(value)) return value;
	if (typeof value === "string" && value.trim() !== "") {
		const parsed = Number(value);
		return Number.isFinite(parsed) ? parsed : null;
	}
	return null;
};

const pick = (raw: Record<string, unknown>, ...keys: string[]): unknown => {
	for (const key of keys) {
		if (raw[key] !== undefined && raw[key] !== null) return raw[key];
	}
	return undefined;
};

const stateFromWire = (
	raw: Record<string, unknown>,
): DynamicStoplossState | null => {
	const labeled = pick(raw, "state", "State");
	if (typeof labeled === "string") {
		if (labeled === "TakeProfit") return "Armed";
		if (STATE_LABELS.has(labeled)) return labeled as DynamicStoplossState;
	}

	const code = num(labeled);
	if (code === null || code < 0 || code > 3) return null;

	return STATE_FROM_CODE[code as StoplossStateCode];
};

/*
normalizeStoploss maps broker.Stoploss wire JSON (PascalCase + iota State)
onto the UI DynamicStoploss shape. Also accepts an already-normalized object.
*/
export const normalizeStoploss = (raw: unknown): DynamicStoploss | null => {
	if (!raw || typeof raw !== "object") return null;

	const row = raw as Record<string, unknown>;
	const state = stateFromWire(row);
	if (state === null) return null;

	const current =
		num(pick(row, "current_pnl", "CurrentPnLPct", "current_pnl_pct")) ?? null;
	const peak = num(pick(row, "peak_pnl", "PeakPnLPct", "peak_pnl_pct")) ?? null;
	const floor =
		num(pick(row, "floor_pnl", "FloorPnLPct", "floor_pnl_pct")) ?? null;
	const initial =
		num(pick(row, "friction", "InitialPnLPct", "initial_pnl_pct")) ?? null;

	if (current === null || peak === null || floor === null) return null;

	// Soft floor is live once Armed (or later): Go trails keep × peak, min 0.5%.
	const hasFloor =
		row.has_floor === true || state === "Armed" || state === "Triggered";

	const span = Math.max(1e-9, peak - floor);
	const room = num(pick(row, "room_pct")) ?? current - floor;
	const proximity =
		num(pick(row, "proximity")) ?? Math.max(0, Math.min(1, 1 - room / span));
	const friction = initial !== null ? Math.abs(initial) : 0;

	const stop: DynamicStoploss = {
		state,
		friction,
		current_pnl: current,
		peak_pnl: peak,
		floor_pnl: floor,
		has_floor: hasFloor,
		room_pct: room,
		proximity,
	};

	const reason = pick(row, "exit_reason");

	if (typeof reason === "string" && reason !== "") {
		stop.exit_reason = reason;
	}

	const opened = pick(row, "opened_at", "PreviousTime");

	if (typeof opened === "string" && opened !== "") {
		stop.opened_at = opened;
	}

	const peakAt = pick(row, "last_peak_time", "CurrentTime");

	if (typeof peakAt === "string" && peakAt !== "") {
		stop.last_peak_time = peakAt;
	}

	return stop;
};
