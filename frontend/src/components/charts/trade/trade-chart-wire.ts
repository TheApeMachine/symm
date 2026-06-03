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

const chartSinks = new Map<string, BarSink>();

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

const hubRowToBar = (row: OhlcHubRow): OhlcBar => {
	const intervalBegin = row.interval_begin ?? "";
	const parsed = Date.parse(intervalBegin);
	const sec =
		Number.isFinite(parsed) && parsed > 0
			? Math.floor(parsed / 1000)
			: Math.floor(Date.now() / 1000);

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

/*
registerTradeChart connects a mounted chart appendBar to candle_bar websocket frames.
ingestCandleWire parses hub payloads and forwards them to the registered sink only.
*/
export const registerTradeChart = (
	symbol: string,
	appendBar: BarSink,
): (() => void) => {
	chartSinks.set(symbol, appendBar);

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

	chartSinks.get(parsed.symbol)?.(parsed.bar);
};
