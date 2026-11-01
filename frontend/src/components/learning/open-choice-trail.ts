import { closeNowReturnPct, decimal } from "#/components/book-readout";

export type ChoiceSample = { t: number; pnl: number; stamp: string };

const trails = new Map<string, ChoiceSample[]>();

/** Ring buffer length shared by ui/Trail on cards and the open-choices chart. */
export const CHOICE_TRAIL_LENGTH = 90;

const finiteNums = (raw: unknown): number[] => {
	if (!Array.isArray(raw)) return [];
	return raw.filter(
		(value): value is number => typeof value === "number" && Number.isFinite(value),
	);
};

const finiteFloor = (raw: unknown): number | undefined =>
	typeof raw === "number" && Number.isFinite(raw) ? raw : undefined;

const nest = (
	row: Record<string, unknown>,
	key: string,
): Record<string, unknown> => {
	const value = row[key];
	return value && typeof value === "object"
		? (value as Record<string, unknown>)
		: {};
};

const structuralOf = (row: Record<string, unknown>) => nest(row, "structural");
const learningOf = (row: Record<string, unknown>) => nest(row, "learning");

/*
adaptiveBeatsOfficial is true when the learning shadow exit (or its live mark)
is strictly better than the official structural exit. That is the only case
where the chart keeps the adaptive path as a dashed "what if" line.
*/
export const adaptiveBeatsOfficial = (
	row: Record<string, unknown>,
): boolean => {
	const official = finiteFloor(row.official_exit_pct);
	if (official === undefined) return false;

	const learning = learningOf(row);

	if (learning.exit_at != null && learning.exit_at !== "") {
		const done = finiteFloor(learning.exit_pct);
		return done !== undefined && done > official;
	}

	const shadow = finiteNums(learning.pnl_trace);
	if (shadow.length === 0) return false;

	const live = shadow[shadow.length - 1];
	return Number.isFinite(live) && live > official;
};

/*
officialExitDashIndex is the first sample after the active stop settled, drawn
dashed only while the adaptive shadow is beating that official exit.
*/
export const officialExitDashIndex = (
	row: Record<string, unknown>,
): number | undefined => {
	if (row.official_exit_at == null || row.official_exit_at === "") {
		return undefined;
	}

	if (!adaptiveBeatsOfficial(row)) return undefined;

	const shadow = finiteNums(learningOf(row).pnl_trace);
	const official = finiteNums(structuralOf(row).pnl_trace);
	if (shadow.length === 0 || official.length === 0) return undefined;
	if (shadow.length <= official.length) return undefined;

	return official.length;
};

/*
noteOpenChoice appends one close-now sample. Call it when the positions store
delivers a new mark, not on a wall-clock timer.
*/
export const noteOpenChoice = (
	id: string,
	pnl: number,
	stamp: string,
	now = Date.now(),
): boolean => {
	if (!Number.isFinite(pnl) || stamp === "") return false;

	const prior = trails.get(id) ?? [];
	const last = prior[prior.length - 1];
	if (last && last.stamp === stamp) return false;

	const next = [...prior, { t: now, pnl, stamp }];
	trails.set(
		id,
		next.length > CHOICE_TRAIL_LENGTH
			? next.slice(next.length - CHOICE_TRAIL_LENGTH)
			: next,
	);

	return true;
};

export const openChoiceTrail = (id: string): ChoiceSample[] =>
	trails.get(id) ?? [];

/** Percent series for ui/Trail and the open-choices chart (same buffer). */
export const openChoiceValues = (id: string): number[] =>
	openChoiceTrail(id).map((sample) => sample.pnl);

const STROKES = [
	"#7eb6ff",
	"#f0b429",
	"#5eead4",
	"#c4b5fd",
	"#e07a7a",
	"#86efac",
	"#f9a8d4",
];

const strokeFor = (id: string) => {
	let hash = 0;
	for (let i = 0; i < id.length; i++) {
		hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
	}
	return STROKES[hash % STROKES.length];
};

export type OpenChoiceSeries = {
	id: string;
	name: string;
	values: number[];
	stroke: string;
	/** First index drawn dashed (adaptive path after official, only if beating). */
	dashFrom?: number;
	shadowActive?: boolean;
	/** Active structural stop floor in PnL % (latest). */
	stopFloor?: number;
	/** Learning shadow floor in PnL % (latest). */
	shadowFloor?: number;
	/** Active floor path aligned to `values` (lower band). */
	stopFloorTrace?: number[];
	/** Learning floor path aligned to `values` (lower band). */
	shadowFloorTrace?: number[];
	/** Absolute close-now PnL in money (USD book). */
	pnlMoney?: number;
	/** 0..1+ toward the active structural stop. */
	stopProximity?: number;
	/** PnL points remaining above the active structural floor. */
	stopRoomPct?: number;
	shadowProximity?: number;
	shadowRoomPct?: number;
};

/** Clip a server path to the same trailing window as the visible PnL series. */
export const clipTraceToValues = (
	trace: number[],
	valueLen: number,
	sourceLen: number,
): number[] => {
	if (trace.length === 0 || valueLen === 0) return [];

	const start = sourceLen > valueLen ? sourceLen - valueLen : 0;
	const clipped = start > 0 ? trace.slice(start) : [...trace];
	if (clipped.length === valueLen) return clipped;
	if (clipped.length > valueLen) return clipped.slice(clipped.length - valueLen);

	const pad = clipped[0] ?? 0;
	return [...Array(valueLen - clipped.length).fill(pad), ...clipped];
};

/** Read stop floors from desk position JSON. */
export const openChoiceFloors = (
	row: Record<string, unknown>,
): { stopFloor?: number; shadowFloor?: number } => {
	const structural = structuralOf(row);
	const learning = learningOf(row);
	const state =
		structural.state && typeof structural.state === "object"
			? (structural.state as Record<string, unknown>)
			: {};

	const stopFloor =
		finiteFloor(structural.floor_pct) ??
		finiteFloor(state.floorPct ?? state.floor_pct);

	return {
		stopFloor,
		shadowFloor: finiteFloor(learning.floor_pct),
	};
};

/** Absolute money PnL from the desk lot (not percent). */
export const openChoicePnlMoney = (
	row: Record<string, unknown>,
): number | undefined => {
	const pnl = decimal(row.pnl);
	return pnl === null ? undefined : pnl;
};

/*
openChoiceFloorBands builds the lower envelopes for the chart band fill.
Active structural floor while the settler is live; learning floor covers the
evaluated path (including post-official continuation when it is beating).
*/
export const openChoiceFloorBands = (
	row: Record<string, unknown>,
	values: number[],
): { stopFloorTrace?: number[]; shadowFloorTrace?: number[] } => {
	const structural = structuralOf(row);
	const learning = learningOf(row);
	const stopRaw = finiteNums(structural.floor_trace);
	const shadowRaw = finiteNums(learning.floor_trace);
	const shadowPnl = finiteNums(learning.pnl_trace);
	const officialPnl = finiteNums(structural.pnl_trace);
	const sourceLen =
		shadowPnl.length > 0 ? shadowPnl.length : Math.max(officialPnl.length, values.length);

	const { stopFloor, shadowFloor } = openChoiceFloors(row);

	const stopFloorTrace =
		stopRaw.length > 0
			? clipTraceToValues(stopRaw, values.length, sourceLen)
			: stopFloor !== undefined && values.length > 0
				? values.map(() => stopFloor)
				: undefined;

	const shadowFloorTrace =
		shadowRaw.length > 0
			? clipTraceToValues(shadowRaw, values.length, sourceLen)
			: shadowFloor !== undefined && values.length > 0
				? values.map(() => shadowFloor)
				: undefined;

	return { stopFloorTrace, shadowFloorTrace };
};

/** Clip a numeric path to the trailing chart window. */
const clipSeriesWindow = (path: number[]): number[] => {
	if (path.length <= CHOICE_TRAIL_LENGTH) return path;
	return path.slice(path.length - CHOICE_TRAIL_LENGTH);
};

/*
openChoiceSeriesValues shows the learning shadow dashed after the official
settle while the shadow is still live, or once it has beaten structural.
A finished shadow that lost stays on the solid official path.
*/
export const openChoiceSeriesValues = (
	row: Record<string, unknown>,
	id: string,
): { values: number[]; dashFrom?: number } => {
	const learning = learningOf(row);
	const shadow = finiteNums(learning.pnl_trace);
	const official = finiteNums(structuralOf(row).pnl_trace);
	const client = openChoiceValues(id);

	if (shadow.length === 0) {
		return { values: official.length > 0 ? clipSeriesWindow(official) : client };
	}

	const settled =
		row.official_exit_at != null && row.official_exit_at !== "";
	const shadowLive = learning.active === true;
	const beats = adaptiveBeatsOfficial(row);

	if (settled && !shadowLive && !beats) {
		const path =
			official.length > 0
				? official
				: shadow.slice(0, official.length || shadow.length);
		return { values: clipSeriesWindow(path.length > 0 ? path : shadow) };
	}

	const rawDash = settled
		? (officialExitDashIndex(row) ??
			(official.length > 0 && shadow.length > official.length
				? official.length
				: undefined))
		: undefined;
	const start =
		shadow.length > CHOICE_TRAIL_LENGTH
			? shadow.length - CHOICE_TRAIL_LENGTH
			: 0;
	const values = start > 0 ? shadow.slice(start) : shadow;
	const clipped = rawDash === undefined ? undefined : rawDash - start;

	return {
		values,
		dashFrom:
			clipped !== undefined && clipped > 0 && clipped < values.length
				? clipped
				: undefined,
	};
};

/** Close-now series for the open-choices chart (stable id order, no SciChart). */
export const openChoiceSeries = (
	lots: Record<string, unknown>[],
): OpenChoiceSeries[] => {
	const mapped = lots.map((row) => {
		const id = String(row.id ?? row.symbol);
		const { values, dashFrom } = openChoiceSeriesValues(row, id);
		const { stopFloor, shadowFloor } = openChoiceFloors(row);
		const { stopFloorTrace, shadowFloorTrace } = openChoiceFloorBands(
			row,
			values,
		);

		return {
			id,
			name: String(row.symbol ?? id),
			values,
			stroke: strokeFor(id),
			dashFrom,
			shadowActive: learningOf(row).active === true,
			stopFloor,
			shadowFloor,
			stopFloorTrace,
			shadowFloorTrace,
			pnlMoney: openChoicePnlMoney(row),
			stopProximity: finiteFloor(structuralOf(row).proximity),
			stopRoomPct: finiteFloor(structuralOf(row).room_pct),
			shadowProximity: finiteFloor(learningOf(row).proximity),
			shadowRoomPct: finiteFloor(learningOf(row).room_pct),
		};
	});

	mapped.sort((a, b) => a.id.localeCompare(b.id));
	return mapped;
};

/** Legend order: hottest |P&L| first. */
export const openChoiceLegend = (
	series: OpenChoiceSeries[],
): OpenChoiceSeries[] =>
	[...series].sort((a, b) => {
		const aLast = a.values.length > 0 ? a.values[a.values.length - 1] : 0;
		const bLast = b.values.length > 0 ? b.values[b.values.length - 1] : 0;
		return Math.abs(bLast) - Math.abs(aLast);
	});

export const dropOpenChoices = (live: Set<string>): boolean => {
	let dropped = false;
	for (const id of trails.keys()) {
		if (!live.has(id)) {
			trails.delete(id);
			dropped = true;
		}
	}
	return dropped;
};

export const resetOpenChoices = () => {
	trails.clear();
};

export type OpenChoiceLot = { id: string; pnl: number | null; stamp: string };

export const openChoiceMarkStamp = (row: Record<string, unknown>): string => {
	const learning = learningOf(row);

	return [
		row.pnl_pct,
		row.mark,
		row.peak_pnl_pct,
		row.exit_vwap,
		row.bars_held,
		Array.isArray(learning.pnl_trace) ? learning.pnl_trace.length : 0,
		learning.active === true ? 1 : 0,
	].join("|");
};

export const openChoiceLotsFromRows = (
	rows: Record<string, unknown>[],
): OpenChoiceLot[] =>
	rows.map((row) => ({
		id: String(row.id ?? row.symbol),
		pnl: closeNowReturnPct(row),
		stamp: openChoiceMarkStamp(row),
	}));

/** Record close-now samples for the open lot set; returns true if trails changed. */
export const syncOpenChoiceLots = (rows: OpenChoiceLot[]): boolean => {
	const live = new Set<string>();
	let dirty = false;
	for (const { id, pnl, stamp } of rows) {
		live.add(id);
		if (pnl !== null && noteOpenChoice(id, pnl, stamp)) dirty = true;
	}
	if (dropOpenChoices(live)) dirty = true;
	return dirty;
};

/*
linePath joins samples in order, the same way ui/Trail does.
Index 0 is the oldest kept sample and the last index is now.
*/
export const linePath = (
	points: { x: number; y: number }[],
): string =>
	points
		.map(
			(point, i) =>
				`${i === 0 ? "M" : "L"}${point.x.toFixed(1)},${point.y.toFixed(1)}`,
		)
		.join(" ");

/*
indexMean right-aligns each lot on a shared sample index so now is the last
slot, then takes the equal-weight return of the lots that have a sample there.
*/
export const indexMean = (series: ChoiceSample[][]): number[] => {
	const maxLen = series.reduce((n, samples) => Math.max(n, samples.length), 0);
	const mean: number[] = [];

	for (let i = 0; i < maxLen; i++) {
		let sum = 0;
		let count = 0;
		for (const samples of series) {
			const idx = i - (maxLen - samples.length);
			if (idx < 0) continue;
			const pnl = samples[idx]?.pnl;
			if (pnl === undefined || !Number.isFinite(pnl)) continue;
			sum += pnl;
			count++;
		}
		mean.push(count > 0 ? sum / count : Number.NaN);
	}

	return mean;
};
