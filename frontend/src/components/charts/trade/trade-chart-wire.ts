export type OhlcCandle = {
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
	volume: number;
};

export type CandleWire = OhlcCandle & {
	symbol: string;
};

type BarSink = (bar: OhlcCandle) => void;

const chartSinks = new Map<string, BarSink>();
const pendingBars = new Map<string, OhlcCandle[]>();

const isCandleWire = (raw: unknown): raw is CandleWire => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		typeof row.symbol === "string" &&
		typeof row.sec === "number" &&
		typeof row.open === "number" &&
		typeof row.high === "number" &&
		typeof row.low === "number" &&
		typeof row.close === "number"
	);
};

const candleFromWire = (wire: CandleWire): OhlcCandle => ({
	sec: wire.sec,
	open: wire.open,
	high: wire.high,
	low: wire.low,
	close: wire.close,
	volume: typeof wire.volume === "number" ? wire.volume : 0,
});

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
registerTradeChart connects a mounted chart appendBar to ohlc websocket frames.
ingestCandleWire routes backend candle rows to the registered sink only.
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
	if (!isCandleWire(raw)) {
		return;
	}

	const bar = candleFromWire(raw);
	const sink = chartSinks.get(raw.symbol);

	if (sink) {
		sink(bar);
		return;
	}

	const queued = pendingBars.get(raw.symbol) ?? [];
	queued.push(bar);

	// Bound the pre-mount buffer: a chart that never mounts (or a symbol the
	// user never opens) must not accumulate bars for the whole session. The
	// visible window is 300 candles; buffering more than that buys nothing.
	if (queued.length > 300) {
		queued.splice(0, queued.length - 300);
	}

	pendingBars.set(raw.symbol, queued);
};
