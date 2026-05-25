import "scichart-financial-tools/register.js";

import {
	CursorModifier,
	DateTimeNumericAxis,
	EAutoRange,
	EDataPointWidthMode,
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
	ChartSeedEvent,
	StatusEvent,
	StatusPosition,
	StopRatchetEvent,
	SymmEvent,
	TradeEnterEvent,
} from "#/lib/symm/events";
import { eventTimeSec } from "#/lib/symm/events";
import {
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

const labelPrecision = (price: number): number => {
	if (price >= 100) return 2;
	if (price >= 1) return 4;
	return 6;
};

export type TradeChartControls = {
	handleEvent: (ev: SymmEvent) => void;
	dispose: () => void;
};

export const drawTradeChart = async (
	rootElement: HTMLDivElement,
	symbol: string,
) => {
	const controller = await SymmChartController.create(rootElement, symbol);

	return {
		sciChartSurface: controller.sciChartSurface,
		controls: {
			handleEvent: (ev: SymmEvent) => controller.handleEvent(ev),
			dispose: () => controller.dispose(),
		} satisfies TradeChartControls,
	};
};

/** SciChart financial chart — OHLC candles + StopLossTakeProfitAnnotation from scichart-financial-tools. */
class SymmChartController {
	private readonly symbol: string;
	private surface: SciChartSurface;
	private ohlc: OhlcDataSeries;
	private candles: FastCandlestickRenderableSeries;
	private xAxis: DateTimeNumericAxis;
	private yAxis: NumericAxis;
	private bucket: CandleBar | null = null;
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
			dataPointWidth: 8,
			dataPointWidthMode: EDataPointWidthMode.Absolute,
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
					this.onCandle(ev as CandleBarEvent);
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

	private onSeed(ev: ChartSeedEvent) {
		const bars =
			ev.bars
				?.filter((bar) => bar.t > 0 && bar.c > 0)
				.map((bar) => ({
					sec: bar.t,
					open: bar.o,
					high: bar.h,
					low: bar.l,
					close: bar.c,
				})) ?? [];
		this.loadBars(bars);
	}

	private onEnter(ev: TradeEnterEvent) {
		const last = ev.last && ev.last > 0 ? ev.last : ev.fill;
		const entrySec = eventTimeSec(ev);
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

	private onCandle(ev: CandleBarEvent) {
		const bar = {
			sec: ev.sec,
			open: ev.open,
			high: ev.high,
			low: ev.low,
			close: ev.close,
		};
		this.upsertBar(bar);
		this.refreshRiskZone();
		this.scheduleFrameVisibleRange(bar.close, bar.high, bar.low);
	}

	private loadBars(bars: CandleBar[]) {
		this.ohlc.clear();
		this.bucket = null;

		for (const bar of bars) {
			this.appendBar(bar);
		}

		const lastBar = bars[bars.length - 1];

		if (!lastBar) {
			return;
		}

		this.latestPrice = lastBar.close;
		this.latestSec = lastBar.sec + CANDLE_SECONDS;
		this.bucket = { ...lastBar };
		this.refreshRiskZone();
		this.scheduleFrameVisibleRange(lastBar.close, lastBar.high, lastBar.low);
	}

	private upsertBar(bar: CandleBar) {
		if (bar.sec <= 0 || bar.close <= 0) {
			return;
		}

		const index = this.findBarIndex(bar.sec);

		if (index >= 0) {
			this.writeBar(index, bar);
		}

		if (index < 0) {
			this.appendBar(bar);
		}

		this.latestPrice = bar.close;
		this.latestSec = bar.sec + CANDLE_SECONDS;
		this.bucket = { ...bar };
	}

	private findBarIndex(sec: number) {
		const nativeX = this.ohlc.getNativeXValues();

		for (let index = this.ohlc.count() - 1; index >= 0; index--) {
			const currentSec = nativeX.get(index);

			if (currentSec === sec) {
				return index;
			}

			if (currentSec < sec) {
				return -1;
			}
		}

		return -1;
	}

	private appendBar(bar: CandleBar) {
		const ohlc = widenFlatOHLC(bar.open, bar.high, bar.low, bar.close);
		this.ohlc.append(bar.sec, ohlc.open, ohlc.high, ohlc.low, ohlc.close);
	}

	private writeBar(index: number, bar: CandleBar) {
		const ohlc = widenFlatOHLC(bar.open, bar.high, bar.low, bar.close);
		this.ohlc.update(index, ohlc.open, ohlc.high, ohlc.low, ohlc.close);
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
