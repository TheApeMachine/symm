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
	StopLossTakeProfitAnnotation,
} from "scichart-financial-tools";

import type { CandleBarEvent, StatusPosition } from "#/lib/symm/events";
import { widenFlatOHLC } from "#/lib/symm/chart-candles";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const CANDLE_SECONDS = 5;
const VISIBLE_SECONDS = 5 * 60;
const FIFO_CAPACITY = 720;
const Y_EXPAND_PAD = 0.12;

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
	if (price >= 100) {
		return 2;
	}

	if (price >= 1) {
		return 4;
	}

	return 6;
}

export type TradeChartInitResult = {
	sciChartSurface: SciChartSurface;
	appendCandle: (bar: CandleBarEvent) => void;
	seedCandles: (bars: CandleBarEvent[]) => void;
	setPosition: (position: StatusPosition) => void;
	clearPosition: () => void;
	ratchetStop: (stop: number) => void;
	dispose: () => void;
};

/** SciChart financial chart — server candle bars with stable follow-live axes. */
class SymmChartController {
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

	private constructor(
		surface: SciChartSurface,
		ohlc: OhlcDataSeries,
		candles: FastCandlestickRenderableSeries,
		xAxis: DateTimeNumericAxis,
		yAxis: NumericAxis,
	) {
		this.surface = surface;
		this.ohlc = ohlc;
		this.candles = candles;
		this.xAxis = xAxis;
		this.yAxis = yAxis;
	}

	static async create(
		rootElement: HTMLDivElement,
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
			dataSeriesName: "OHLC",
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
			new RolloverModifier({ showRollover: true }),
			new CursorModifier({ showTooltip: true, showAxisLabels: true }),
			new ZoomPanModifier(),
			new MouseWheelZoomModifier(),
			new ZoomExtentsModifier(),
		);

		const controller = new SymmChartController(
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

	seedCandles(bars: CandleBarEvent[]) {
		this.ohlc.clear();
		this.bucket = null;

		for (const bar of bars) {
			this.ingestCandle(bar, false);
		}

		const last = bars.at(-1);

		if (!last) {
			return;
		}

		this.fitVisibleRange(last.close, last.high, last.low, last.sec);
		this.surface.invalidateElement();
	}

	appendCandle(bar: CandleBarEvent) {
		if (bar.close <= 0 || bar.sec <= 0) {
			return;
		}

		const isUpdate = this.ingestCandle(bar, true);

		if (!isUpdate && this.followLive) {
			this.scrollXToLive(bar.sec);
			this.expandYIfNeeded(bar.high, bar.low, bar.close);
		}

		this.refreshRiskZone();
		this.surface.invalidateElement();
	}

	setPosition(position: StatusPosition) {
		const last =
			(position.last_price ?? 0) > 0
				? (position.last_price as number)
				: position.peak_price > 0
					? position.peak_price
					: position.entry_price;
		const openedSec = position.opened_at
			? Math.floor(Date.parse(position.opened_at) / 1000)
			: Math.floor(Date.now() / 1000) - 300;

		this.latestPrice = last;
		this.setRiskZone(openedSec, position.entry_price, position.stop_price);
		this.surface.invalidateElement();
	}

	clearPosition() {
		this.clearRiskZone();
		this.surface.invalidateElement();
	}

	ratchetStop(stop: number) {
		if (stop <= 0) {
			return;
		}

		this.tradeStop = stop;
		this.refreshRiskZone();
		this.surface.invalidateElement();
	}

	dispose() {
		this.interactionAbort.abort();
		this.clearRiskZone();
	}

	private ingestCandle(bar: CandleBarEvent, animate: boolean): boolean {
		const ohlc = widenFlatOHLC(bar.open, bar.high, bar.low, bar.close);
		this.latestPrice = bar.close;
		this.latestSec = bar.sec;
		this.bucket = {
			sec: bar.sec,
			open: ohlc.open,
			high: ohlc.high,
			low: ohlc.low,
			close: ohlc.close,
		};

		if (this.ohlc.count() > 0) {
			const nativeX = this.ohlc.getNativeXValues();
			const lastIndex = this.ohlc.count() - 1;
			const lastSec = nativeX.get(lastIndex);

			if (lastSec > bar.sec) {
				return true;
			}

			if (lastSec === bar.sec) {
				this.writeBucket(lastIndex);

				return true;
			}
		}

		this.appendBucket(bar.sec);

		if (!animate) {
			return false;
		}

		return false;
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
		if (entry <= 0 || stop <= 0) {
			return;
		}

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
					{ text: "entry", pointIndex: 0 },
					{ text: "stop", pointIndex: 1 },
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
				this.fitVisibleRange(
					this.latestPrice,
					this.bucket?.high ?? this.latestPrice,
					this.bucket?.low ?? this.latestPrice,
					this.latestSec,
				);
				this.surface.invalidateElement();
			},
			{ passive: true, signal },
		);
	}

	private scrollXToLive(sec: number) {
		const anchor = sec + CANDLE_SECONDS;
		this.xAxis.visibleRange = new NumberRange(
			anchor - VISIBLE_SECONDS,
			anchor + CANDLE_SECONDS * 2,
		);
	}

	private expandYIfNeeded(high: number, low: number, close: number) {
		const current = this.yAxis.visibleRange;

		if (!current) {
			this.fitVisibleRange(close, high, low, this.latestSec);
			return;
		}

		const prices = [high, low, close, this.tradeEntry, this.tradeStop].filter(
			(value) => Number.isFinite(value) && value > 0,
		);

		if (prices.length === 0) {
			return;
		}

		const min = Math.min(...prices);
		const max = Math.max(...prices);
		const span = Math.max(max - min, Math.abs(close) * 1e-4, 1e-8);
		const pad = span * Y_EXPAND_PAD;

		if (min >= current.min + pad && max <= current.max - pad) {
			return;
		}

		const nextMin = Math.min(current.min, min - pad);
		const nextMax = Math.max(current.max, max + pad);
		this.setYAxisPrecision((nextMin + nextMax) / 2);
		this.yAxis.visibleRange = new NumberRange(nextMin, nextMax);
	}

	private fitVisibleRange(
		close: number,
		high: number,
		low: number,
		sec: number,
	) {
		if (!this.followLive) {
			return;
		}

		this.scrollXToLive(sec);

		const prices = [high, low, close, this.tradeEntry, this.tradeStop].filter(
			(value) => Number.isFinite(value) && value > 0,
		);

		if (prices.length === 0) {
			return;
		}

		const min = Math.min(...prices);
		const max = Math.max(...prices);
		const mid = (min + max) / 2;
		const span = Math.max(max - min, Math.abs(mid) * 1e-4, 1e-8);
		const pad = span * Y_EXPAND_PAD;

		this.setYAxisPrecision(mid);
		this.yAxis.visibleRange = new NumberRange(min - pad, max + pad);
	}

	private setYAxisPrecision(price: number) {
		const next = labelPrecision(price);
		const labelProvider = this.yAxis.labelProvider;

		if (labelProvider.precision !== next) {
			labelProvider.precision = next;
		}
	}
}

export async function initTradeChart(
	rootElement: string | HTMLDivElement,
): Promise<TradeChartInitResult> {
	if (typeof rootElement === "string") {
		throw new Error("initTradeChart requires an HTMLDivElement root");
	}

	const controller = await SymmChartController.create(rootElement);

	return {
		sciChartSurface: controller.sciChartSurface,
		appendCandle: (bar) => controller.appendCandle(bar),
		seedCandles: (bars) => controller.seedCandles(bars),
		setPosition: (position) => controller.setPosition(position),
		clearPosition: () => controller.clearPosition(),
		ratchetStop: (stop) => controller.ratchetStop(stop),
		dispose: () => controller.dispose(),
	};
}
