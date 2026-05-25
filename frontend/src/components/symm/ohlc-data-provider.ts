export type OhlcBar = {
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
	volume: number;
};

export type OhlcRow = {
	symbol: string;
	open: number;
	high: number;
	low: number;
	close: number;
	volume?: number;
	interval_begin?: string;
};

export type OhlcBootstrapRequest = {
	symbol: string;
	interval?: number;
	startDate?: Date;
	count?: number;
};

export type OhlcVArrays = {
	xValues: number[];
	openValues: number[];
	highValues: number[];
	lowValues: number[];
	closeValues: number[];
	volumeValues: number[];
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

export const isOhlcBootstrapRequest = (
	raw: unknown,
): raw is OhlcBootstrapRequest => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		typeof row.symbol === "string" &&
		typeof row.open !== "number" &&
		(row.count !== undefined ||
			row.startDate !== undefined ||
			row.interval !== undefined)
	);
};

export const isOhlcRow = (raw: unknown): raw is OhlcRow => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		typeof row.symbol === "string" &&
		typeof row.open === "number" &&
		typeof row.high === "number" &&
		typeof row.low === "number" &&
		typeof row.close === "number"
	);
};

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

export const ohlcRowToBar = (row: OhlcRow): OhlcBar => {
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

const candleBarEventToBar = (event: CandleBarEvent): OhlcBar => ({
	sec: event.sec,
	open: event.open,
	high: event.high,
	low: event.low,
	close: event.close,
	volume: event.volume,
});

const barsToArrays = (bars: OhlcBar[]): OhlcVArrays => {
	const xValues: number[] = [];
	const openValues: number[] = [];
	const highValues: number[] = [];
	const lowValues: number[] = [];
	const closeValues: number[] = [];
	const volumeValues: number[] = [];

	for (const bar of bars) {
		xValues.push(bar.sec);
		openValues.push(bar.open);
		highValues.push(bar.high);
		lowValues.push(bar.low);
		closeValues.push(bar.close);
		volumeValues.push(bar.volume);
	}

	return {
		xValues,
		openValues,
		highValues,
		lowValues,
		closeValues,
		volumeValues,
	};
};

type SymbolSink = (bar: OhlcBar) => void;

const emptyArrays = (): OhlcVArrays => ({
	xValues: [],
	openValues: [],
	highValues: [],
	lowValues: [],
	closeValues: [],
	volumeValues: [],
});

/*
OhlcDataProvider feeds SciChart financial charts.

ingest() accepts either a chart bootstrap request (returns OHLCV arrays) or a
live hub row (updates history and registered symbol sinks).
*/
class OhlcDataProviderImpl {
	private sinks = new Map<string, SymbolSink>();
	private history = new Map<string, OhlcBar[]>();

	bootstrap(request: OhlcBootstrapRequest): OhlcVArrays {
		const count = request.count ?? 500;
		const interval = request.interval ?? 3600;
		const startDate =
			request.startDate ?? new Date(Date.now() - count * interval * 1000);
		const startSec = Math.floor(startDate.getTime() / 1000);

		let bars = [...(this.history.get(request.symbol) ?? [])].filter(
			(bar) => bar.sec >= startSec,
		);

		if (bars.length > count) {
			bars = bars.slice(-count);
		}

		if (bars.length === 0) {
			return emptyArrays();
		}

		return barsToArrays(bars);
	}

	getRandomOHLCVData(
		count: number,
		_startPrice: number,
		startDate: Date,
		interval: number,
		..._rest: unknown[]
	): OhlcVArrays {
		return this.bootstrap({
			symbol: "BTC/EUR",
			count,
			startDate,
			interval,
		});
	}

	registerSymbol(symbol: string, sink: SymbolSink): () => void {
		this.sinks.set(symbol, sink);

		for (const bar of this.history.get(symbol) ?? []) {
			sink(bar);
		}

		return () => {
			this.sinks.delete(symbol);
		};
	}

	private upsertBar(symbol: string, bar: OhlcBar): void {
		const bars = this.history.get(symbol) ?? [];
		const last = bars.at(-1);

		if (last !== undefined && last.sec === bar.sec) {
			bars[bars.length - 1] = bar;
		} else {
			bars.push(bar);
		}

		this.history.set(symbol, bars);
		this.sinks.get(symbol)?.(bar);
	}

	ingestHub(raw: unknown): void {
		if (isCandleBarEvent(raw)) {
			this.upsertBar(raw.symbol, candleBarEventToBar(raw));
			return;
		}

		if (!isOhlcRow(raw)) {
			return;
		}

		this.upsertBar(raw.symbol, ohlcRowToBar(raw));
	}

	ingest(raw: unknown): OhlcVArrays | void {
		if (isOhlcBootstrapRequest(raw)) {
			return this.bootstrap(raw);
		}

		this.ingestHub(raw);
	}
}

const shared = new OhlcDataProviderImpl();

type OhlcDataProviderApi = {
	getRandomOHLCVData: (
		count: number,
		startPrice: number,
		startDate: Date,
		interval: number,
		...rest: unknown[]
	) => OhlcVArrays;
	registerSymbol: (symbol: string, sink: SymbolSink) => () => void;
	ingest: {
		(raw: OhlcBootstrapRequest): OhlcVArrays;
		(raw: unknown): void;
	};
};

export const OhlcDataProvider: OhlcDataProviderApi = {
	getRandomOHLCVData: (
		count: number,
		startPrice: number,
		startDate: Date,
		interval: number,
		...rest: unknown[]
	) =>
		shared.getRandomOHLCVData(count, startPrice, startDate, interval, ...rest),
	registerSymbol: (symbol: string, sink: SymbolSink) =>
		shared.registerSymbol(symbol, sink),
	ingest: ((raw: unknown) =>
		shared.ingest(raw)) as OhlcDataProviderApi["ingest"],
};
