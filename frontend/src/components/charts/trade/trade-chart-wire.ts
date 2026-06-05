export type OhlcBar = {
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
	volume: number;
};

type CandleBarEvent = {
	event: "candle_bar";
	symbol: string;
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
	volume: number;
};

type OhlcHubRow = {
	symbol: string;
	open: number;
	high: number;
	low: number;
	close: number;
	volume?: number;
	interval_begin?: string;
};

type BarSink = (bar: OhlcBar) => void;

// Threshold used to identify millisecond timestamps: 1e12 ms is approximately Sep 2001.
const MILLISECOND_TIMESTAMP_THRESHOLD = 1_000_000_000_000;

const chartSinks = new Map<string, BarSink>();
const pendingBars = new Map<string, OhlcBar[]>();

const isCandleBarEvent = (raw: unknown): raw is CandleBarEvent => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		row.event === "candle_bar" &&
		typeof row.symbol === "string" &&
		typeof row.sec === "number" &&
		typeof row.open === "number" &&
		typeof row.high === "number" &&
		typeof row.low === "number" &&
		typeof row.close === "number"
	);
};

const isOhlcHubRow = (raw: unknown): raw is OhlcHubRow => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	if (typeof row.event === "string") {
		return false;
	}

	return (
		typeof row.symbol === "string" &&
		typeof row.open === "number" &&
		typeof row.high === "number" &&
		typeof row.low === "number" &&
		typeof row.close === "number"
	);
};

export const parseIntervalBeginSec = (
	intervalBegin: unknown,
): number | null => {
	if (typeof intervalBegin === "number" && Number.isFinite(intervalBegin)) {
		if (intervalBegin > MILLISECOND_TIMESTAMP_THRESHOLD) {
			return Math.floor(intervalBegin / 1000);
		}

		if (intervalBegin > 0) {
			return Math.floor(intervalBegin);
		}

		return null;
	}

	if (typeof intervalBegin !== "string") {
		return null;
	}

	const trimmed = intervalBegin.trim();

	if (trimmed.length === 0) {
		return null;
	}

	const parsed = Date.parse(trimmed);

	if (!Number.isFinite(parsed) || parsed <= 0) {
		return null;
	}

	return Math.floor(parsed / 1000);
};

const hubRowToBar = (row: OhlcHubRow): OhlcBar => {
	const parsedSec = parseIntervalBeginSec(row.interval_begin);
	const sec = parsedSec ?? Math.floor(Date.now() / 1000);

	return {
		sec,
		open: row.open,
		high: row.high,
		low: row.low,
		close: row.close,
		volume: row.volume ?? 0,
	};
};

export const parseCandleWire = (
	raw: unknown,
): { symbol: string; bar: OhlcBar } | undefined => {
	if (isCandleBarEvent(raw)) {
		return {
			symbol: raw.symbol,
			bar: {
				sec: raw.sec,
				open: raw.open,
				high: raw.high,
				low: raw.low,
				close: raw.close,
				volume: raw.volume,
			},
		};
	}

	if (!isOhlcHubRow(raw)) {
		return undefined;
	}

	return {
		symbol: raw.symbol,
		bar: hubRowToBar(raw),
	};
};

const flushPending = (symbol: string, sink: BarSink): void => {
	const queued = pendingBars.get(symbol);

	if (!queued) {
		return;
	}

	for (const bar of queued) {
		sink(bar);
	}

	pendingBars.delete(symbol);
};

/*
registerTradeChart connects a mounted chart appendBar to candle_bar websocket frames.
ingestCandleWire parses hub payloads and forwards them to the registered sink only.
*/
export const registerTradeChart = (
	symbol: string,
	appendBar: BarSink,
): (() => void) => {
	chartSinks.set(symbol, appendBar);
	flushPending(symbol, appendBar);

	return () => {
		if (chartSinks.get(symbol) === appendBar) {
			chartSinks.delete(symbol);
		}
	};
};

export const ingestCandleWire = (raw: unknown): void => {
	const parsed = parseCandleWire(raw);

	if (parsed === undefined) {
		return;
	}

	const sink = chartSinks.get(parsed.symbol);

	if (sink) {
		sink(parsed.bar);
		return;
	}

	const queued = pendingBars.get(parsed.symbol) ?? [];
	queued.push(parsed.bar);
	pendingBars.set(parsed.symbol, queued);
};
