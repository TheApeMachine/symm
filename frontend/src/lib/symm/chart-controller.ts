import "scichart-financial-tools/register.js";

import {
	CursorModifier,
	DateTimeNumericAxis,
	EAutoRange,
	ENumericFormat,
	FastCandlestickRenderableSeries,
	MouseWheelZoomModifier,
	NumberRange,
	NumericAxis,
	OhlcDataSeries,
	RolloverModifier,
	SciChartSurface,
	ZoomExtentsModifier,
	ZoomPanModifier,
} from "scichart";
import {
	EAnnotationVisibilityMode,
	ESnapMode,
	SeriesValueModifier,
	StopLossTakeProfitAnnotation,
} from "scichart-financial-tools";

import type {
	CandleBarEvent,
	ChartReplayEvent,
	ChartSeedEvent,
	PriceTickEvent,
	StatusEvent,
	StatusPosition,
	StopRatchetEvent,
	SymmEvent,
	TradeEnterEvent,
} from "#/lib/symm/events";
import { eventTimeSec } from "#/lib/symm/events";
import {
	aggregateTicksToCandles,
	type CandleBar,
	widenFlatOHLC,
} from "#/lib/symm/chart-candles";
import { candleYRange } from "#/lib/symm/chart-range";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const CANDLE_SECONDS = 5;
const VISIBLE_SECONDS = 5 * 60;
const FIFO_CAPACITY = 720;

const TAKE_PROFIT_COLOR = "#16A34A";
const STOP_LOSS_COLOR = "#EF4444";

type Bucket = {
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
};

function labelPrecision(price: number): number {
	if (price >= 100) return 2;
	if (price >= 1) return 4;
	return 6;
}

function eventSecond(raw: string | undefined): number {
	const ms = Date.parse(raw ?? "");
	return Number.isFinite(ms) ? ms / 1000 : Date.now() / 1000;
}

function tickSecond(ev: PriceTickEvent): number {
	return eventSecond(ev.at || ev.ts);
}

export type TradeChartInitResult = {
	sciChartSurface: SciChartSurface;
	handleEvent: (ev: SymmEvent) => void;
	dispose: () => void;
};

/** SciChart financial chart — OHLC candles + StopLossTakeProfitAnnotation from scichart-financial-tools. */
class SymmChartController {
	private readonly symbol: string;
	private surface: SciChartSurface;
	private ohlc: OhlcDataSeries;
	private candles: FastCandlestickRenderableSeries;
	private xAxis: DateTimeNumericAxis;
	private yAxis: NumericAxis;
	private bucket: Bucket | null = null;
	private latestPrice = 0;
	private latestSec = 0;
	private tradeEntrySec = 0;
	private tradeEntry = 0;
	private tradeStop = 0;
	private riskZone: StopLossTakeProfitAnnotation | null = null;
	private followLive = true;
	private readonly interactionAbort = new AbortController();
	private frameScheduled = false;
	private frameHandle = 0;
	private disposed = false;
	private pendingFramePrices: number[] = [];

	private constructor(
		symbol: string,
		surface: SciChartSurface,
		ohlc: OhlcDataSeries,
		candles: FastCandlestickRenderableSeries,
		xAxis: DateTimeNumericAxis,
		yAxis: NumericAxis,
	) {
		this.symbol = symbol;
		this.surface = surface;
		this.ohlc = ohlc;
		this.candles = candles;
		this.xAxis = xAxis;
		this.yAxis = yAxis;
	}

	static async create(
		rootElement: HTMLDivElement,
		symbol: string,
	): Promise<SymmChartController> {
		await ensureSciChartWasm();
		const { sciChartSurface, wasmContext } =
			await SciChartSurface.create(rootElement);
		sciChartSurface.title = "";

		const xAxis = new DateTimeNumericAxis(wasmContext, {
			autoRange: EAutoRange.Never,
			growBy: new NumberRange(0.02, 0.04),
		});
		const yAxis = new NumericAxis(wasmContext, {
			autoRange: EAutoRange.Never,
			growBy: new NumberRange(0.08, 0.12),
			labelFormat: ENumericFormat.Decimal,
			labelPrecision: 6,
		});
		const ohlc = new OhlcDataSeries(wasmContext, {
			dataSeriesName: symbol,
			dataIsSortedInX: true,
			containsNaN: false,
			fifoCapacity: FIFO_CAPACITY,
		});
		const candles = new FastCandlestickRenderableSeries(wasmContext, {
			dataSeries: ohlc,
			dataPointWidth: 0.7,
			brushUp: "#22C55E",
			brushDown: "#EF4444",
		});

		sciChartSurface.xAxes.add(xAxis);
		sciChartSurface.yAxes.add(yAxis);
		sciChartSurface.renderableSeries.add(candles);
		sciChartSurface.chartModifiers.add(
			new RolloverModifier({ showRolloverLine: true }),
			new CursorModifier({ showTooltip: true, showAxisLabels: true }),
			new SeriesValueModifier(),
			new ZoomPanModifier(),
			new MouseWheelZoomModifier(),
			new ZoomExtentsModifier(),
		);

		const controller = new SymmChartController(
			symbol,
			sciChartSurface,
			ohlc,
			candles,
			xAxis,
			yAxis,
		);
		controller.bindUserNavigation(rootElement);
		return controller;
	}

	get sciChartSurface(): SciChartSurface {
		return this.surface;
	}

	syncPosition(pos: StatusPosition) {
		const last =
			(pos.last_price ?? 0) > 0
				? (pos.last_price as number)
				: pos.peak_price > 0
					? pos.peak_price
					: pos.entry_price;
		const openedSec = pos.opened_at
			? Math.floor(Date.parse(pos.opened_at) / 1000)
			: Math.floor(Date.now() / 1000) - 300;

		this.setRiskZone(openedSec, pos.entry_price, pos.stop_price);
		this.scheduleFrameVisibleRange(last);
	}

	handleEvent(ev: SymmEvent) {
		if (this.disposed) {
			return;
		}

		switch (ev.event) {
			case "candle_bar":
				if (ev.symbol === this.symbol) {
					this.onCandleBar(ev as CandleBarEvent);
				}
				return;
			case "price_tick":
				if (ev.symbol === this.symbol) this.onTick(ev as PriceTickEvent);
				return;
			case "chart_replay":
				if (ev.symbol === this.symbol) {
					this.onReplay(ev as ChartReplayEvent);
				}
				return;
			case "stop_ratchet":
				if (ev.symbol === this.symbol) this.onRatchet(ev as StopRatchetEvent);
				return;
			case "trade_enter":
				if (ev.symbol === this.symbol) this.onEnter(ev as TradeEnterEvent);
				return;
			case "trade_exit":
				if (ev.symbol === this.symbol) this.onExit();
				return;
			case "status":
				this.onStatus(ev as StatusEvent);
				return;
			case "chart_seed":
				if (ev.symbol === this.symbol) this.onSeed(ev as ChartSeedEvent);
				return;
			case "scoreboard":
				return;
		}
	}

	dispose() {
		this.disposed = true;

		if (this.frameHandle !== 0) {
			cancelAnimationFrame(this.frameHandle);
			this.frameHandle = 0;
		}

		this.frameScheduled = false;
		this.interactionAbort.abort();
		this.clearRiskZone();
	}

	private onCandleBar(ev: CandleBarEvent) {
		if (ev.close <= 0 || ev.sec <= 0) {
			return;
		}

		const ohlc = widenFlatOHLC(ev.open, ev.high, ev.low, ev.close);
		this.latestPrice = ev.close;
		this.latestSec = ev.sec;
		this.bucket = {
			sec: ev.sec,
			open: ohlc.open,
			high: ohlc.high,
			low: ohlc.low,
			close: ohlc.close,
		};

		if (this.ohlc.count() > 0) {
			const nativeX = this.ohlc.getNativeXValues();
			const lastIndex = this.ohlc.count() - 1;
			const lastSec = nativeX.get(lastIndex);

			if (lastSec > ev.sec) {
				return;
			}

			if (lastSec === ev.sec) {
				this.writeBucket(lastIndex);
				this.refreshRiskZone();
				this.scheduleFrameVisibleRange(ev.close, ev.high, ev.low);
				return;
			}
		}

		this.appendBucket(ev.sec);
		this.refreshRiskZone();
		this.scheduleFrameVisibleRange(ev.close, ev.high, ev.low);
	}

	private onTick(ev: PriceTickEvent) {
		if (ev.last <= 0) {
			return;
		}

		this.latestPrice = ev.last;
		this.latestSec = tickSecond(ev);
		this.refreshRiskZone();
	}

	private onReplay(ev: ChartReplayEvent) {
		this.loadBars(aggregateTicksToCandles(ev.ticks, CANDLE_SECONDS));
	}

	private onSeed(ev: ChartSeedEvent) {
		const bars = ev.bars?.filter((bar) => bar.t > 0 && bar.c > 0) ?? [];
		if (bars.length === 0) return;
		this.ohlc.clear();
		this.bucket = null;
		for (const bar of bars) {
			const ohlc = widenFlatOHLC(bar.o, bar.h, bar.l, bar.c);
			this.ohlc.append(bar.t, ohlc.open, ohlc.high, ohlc.low, ohlc.close);
		}
		const last = bars[bars.length - 1];
		this.latestPrice = last.c;
		this.latestSec = last.t;
		this.bucket = {
			sec: Math.floor(last.t / CANDLE_SECONDS) * CANDLE_SECONDS,
			open: last.o,
			high: last.h,
			low: last.l,
			close: last.c,
		};
		this.refreshRiskZone();
		this.scheduleFrameVisibleRange(last.c, last.h, last.l);
	}

	private onEnter(ev: TradeEnterEvent) {
		const last = ev.last && ev.last > 0 ? ev.last : ev.fill;
		const entrySec = eventTimeSec(ev);
		if (last > 0 && this.ohlc.count() === 0) {
			this.appendPrice(last, entrySec);
		}
		this.setRiskZone(entrySec, ev.fill, ev.stop);
		this.scheduleFrameVisibleRange(last);
	}

	private onRatchet(ev: StopRatchetEvent) {
		this.tradeStop = ev.new_stop;
		this.refreshRiskZone();
		this.scheduleFrameVisibleRange(ev.last);
	}

	private onExit() {
		this.clearRiskZone();
	}

	private onStatus(ev: StatusEvent) {
		const pos = ev.positions?.find((p) => p.symbol === this.symbol);
		if (pos) this.syncPosition(pos);
		else this.clearRiskZone();
	}

	private appendPrice(last: number, sec: number) {
		const nextSec = Number.isFinite(sec) ? sec : Date.now() / 1000;
		const bucketSec = Math.floor(nextSec / CANDLE_SECONDS) * CANDLE_SECONDS;
		this.latestPrice = last;
		this.latestSec = nextSec;

		if (this.bucket && this.bucket.sec === bucketSec && this.ohlc.count() > 0) {
			this.bucket.high = Math.max(this.bucket.high, last);
			this.bucket.low = Math.min(this.bucket.low, last);
			this.bucket.close = last;
			this.writeBucket(this.ohlc.count() - 1);
		} else if (this.ohlc.count() > 0) {
			const nativeX = this.ohlc.getNativeXValues();
			const lastIdx = this.ohlc.count() - 1;
			const lastX = nativeX.get(lastIdx);

			if (bucketSec === lastX) {
				this.bucket = {
					sec: bucketSec,
					open: this.bucket?.open ?? last,
					high: Math.max(this.bucket?.high ?? last, last),
					low: Math.min(this.bucket?.low ?? last, last),
					close: last,
				};
				this.writeBucket(lastIdx);
			} else if (bucketSec > lastX) {
				this.bucket = {
					sec: bucketSec,
					open: last,
					high: last,
					low: last,
					close: last,
				};
				this.appendBucket(bucketSec);
			}
		} else {
			this.bucket = {
				sec: bucketSec,
				open: last,
				high: last,
				low: last,
				close: last,
			};
			this.appendBucket(bucketSec);
		}

		this.refreshRiskZone();
		this.scheduleFrameVisibleRange(last);
	}

	private loadBars(bars: CandleBar[]) {
		this.ohlc.clear();
		this.bucket = null;

		for (const bar of bars) {
			const ohlc = widenFlatOHLC(bar.open, bar.high, bar.low, bar.close);
			this.ohlc.append(bar.sec, ohlc.open, ohlc.high, ohlc.low, ohlc.close);
		}

		const lastBar = bars[bars.length - 1];

		if (!lastBar) {
			return;
		}

		this.latestPrice = lastBar.close;
		this.latestSec = lastBar.sec + CANDLE_SECONDS - 1;
		this.bucket = { ...lastBar };
		this.refreshRiskZone();
		this.scheduleFrameVisibleRange(lastBar.close, lastBar.high, lastBar.low);
	}

	private writeBucket(index: number) {
		if (!this.bucket) {
			return;
		}

		const ohlc = widenFlatOHLC(
			this.bucket.open,
			this.bucket.high,
			this.bucket.low,
			this.bucket.close,
		);
		this.ohlc.update(index, ohlc.open, ohlc.high, ohlc.low, ohlc.close);
	}

	private appendBucket(bucketSec: number) {
		if (!this.bucket) {
			return;
		}

		const ohlc = widenFlatOHLC(
			this.bucket.open,
			this.bucket.high,
			this.bucket.low,
			this.bucket.close,
		);
		this.ohlc.append(bucketSec, ohlc.open, ohlc.high, ohlc.low, ohlc.close);
	}

	private setRiskZone(entrySec: number, entry: number, stop: number) {
		if (entry <= 0 || stop <= 0) return;

		this.tradeEntrySec = entrySec;
		this.tradeEntry = entry;
		this.tradeStop = stop;

		if (!this.riskZone) {
			this.riskZone = new StopLossTakeProfitAnnotation({
				strokeThickness: 2,
				strokeDashArray: [6, 3],
				takeProfitColor: TAKE_PROFIT_COLOR,
				stopLossColor: STOP_LOSS_COLOR,
				fillOpacity: 0.18,
				axisLabelVisibility: EAnnotationVisibilityMode.Hidden,
				axisSpanFillOpacity: 0,
				isEditable: false,
				snapMode: ESnapMode.None,
				labels: [
					{
						text: "entry",
						pointIndex: 0,
					},
					{
						text: "stop",
						pointIndex: 1,
					},
				],
			});
			this.surface.annotations.add(this.riskZone);
		}

		this.refreshRiskZone();
	}

	private refreshRiskZone() {
		if (
			!this.riskZone ||
			this.tradeEntrySec <= 0 ||
			this.tradeEntry <= 0 ||
			this.tradeStop <= 0
		) {
			return;
		}
		const endSec = Math.max(
			this.latestSec || Date.now() / 1000,
			this.tradeEntrySec + CANDLE_SECONDS,
		);
		this.riskZone.points = [
			{ x: this.tradeEntrySec, y: this.tradeEntry },
			{ x: endSec, y: this.tradeStop },
		];
	}

	private clearRiskZone() {
		if (this.riskZone) {
			this.surface.annotations.remove(this.riskZone, true);
			this.riskZone = null;
		}
		this.tradeEntrySec = 0;
		this.tradeEntry = 0;
		this.tradeStop = 0;
	}

	private bindUserNavigation(rootElement: HTMLDivElement) {
		const signal = this.interactionAbort.signal;
		const leaveFollowMode = () => {
			this.followLive = false;
		};
		rootElement.addEventListener("wheel", leaveFollowMode, {
			passive: true,
			signal,
		});
		rootElement.addEventListener("pointerdown", leaveFollowMode, {
			passive: true,
			signal,
		});
		rootElement.addEventListener(
			"dblclick",
			() => {
				this.followLive = true;
				this.scheduleFrameVisibleRange(this.latestPrice);
			},
			{ passive: true, signal },
		);
	}

	private scheduleFrameVisibleRange(...prices: number[]) {
		if (this.disposed) {
			return;
		}

		this.pendingFramePrices = prices;

		if (this.frameScheduled) {
			return;
		}

		this.frameScheduled = true;

		this.frameHandle = requestAnimationFrame(() => {
			this.frameHandle = 0;
			this.frameScheduled = false;

			if (this.disposed) {
				return;
			}

			this.applyFrameVisibleRange(...this.pendingFramePrices);
			this.surface.invalidateElement();
		});
	}

	private applyFrameVisibleRange(...prices: number[]) {
		if (this.disposed || !this.followLive) {
			return;
		}

		const latest = this.latestSec || Date.now() / 1000;

		this.xAxis.visibleRange = new NumberRange(
			latest - VISIBLE_SECONDS,
			latest + CANDLE_SECONDS * 2,
		);

		const livePrices = [
			...prices,
			this.latestPrice,
			this.bucket?.high,
			this.bucket?.low,
		].filter(
			(value): value is number =>
				value !== undefined && Number.isFinite(value) && value > 0,
		);

		let seriesMin = Number.NaN;
		let seriesMax = Number.NaN;

		if (this.ohlc.count() > 0) {
			const xRange = this.xAxis.visibleRange;
			const seriesYRange = this.candles.getYRange(xRange, false);
			seriesMin = seriesYRange.min;
			seriesMax = seriesYRange.max;
		}

		const nextRange = candleYRange(seriesMin, seriesMax, livePrices);

		if (!nextRange) {
			return;
		}

		const mid = (nextRange.min + nextRange.max) / 2;
		this.setYAxisPrecision(mid);
		this.yAxis.visibleRange = new NumberRange(nextRange.min, nextRange.max);
	}

	private setYAxisPrecision(price: number) {
		const next = labelPrecision(price);
		const lp = this.yAxis.labelProvider;
		if (lp.precision !== next) {
			lp.precision = next;
		}
	}
}

export async function initTradeChart(
	rootElement: string | HTMLDivElement,
	symbol: string,
): Promise<TradeChartInitResult> {
	if (typeof rootElement === "string") {
		throw new Error("initTradeChart requires an HTMLDivElement root");
	}
	const controller = await SymmChartController.create(rootElement, symbol);
	return {
		sciChartSurface: controller.sciChartSurface,
		handleEvent: (ev) => controller.handleEvent(ev),
		dispose: () => controller.dispose(),
	};
}
