import type {
	CandleBarEvent,
	ChartSeedEvent,
	SymmEvent,
} from "#/lib/symm/events";
import type { CandleBar } from "#/lib/symm/chart-candles";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";
import {
	createFinancialChartSurface,
	followLatestCandleRange,
	refreshFinancialPriceAxis,
	type FinancialChartContext,
} from "#/components/symm/financial-chart-utils";

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

class SymmChartController {
	private readonly symbol: string;
	private readonly chart: FinancialChartContext;
	private disposed = false;

	private constructor(symbol: string, chart: FinancialChartContext) {
		this.symbol = symbol;
		this.chart = chart;
	}

	static async create(
		rootElement: HTMLDivElement,
		symbol: string,
	): Promise<SymmChartController> {
		await ensureSciChartWasm();

		const chart = await createFinancialChartSurface(rootElement, symbol);

		return new SymmChartController(symbol, chart);
	}

	get sciChartSurface() {
		return this.chart.sciChartSurface;
	}

	handleEvent(ev: SymmEvent) {
		if (this.disposed) {
			return;
		}

		if (ev.event === "candle_bar" && ev.symbol === this.symbol) {
			this.onCandle(ev as CandleBarEvent);
			return;
		}

		if (ev.event === "chart_seed" && ev.symbol === this.symbol) {
			this.onSeed(ev as ChartSeedEvent);
		}
	}

	dispose() {
		this.disposed = true;
	}

	private onSeed(ev: ChartSeedEvent) {
		const bars =
			ev.bars?.map((bar) => ({
				sec: bar.t,
				open: bar.o,
				high: bar.h,
				low: bar.l,
				close: bar.c,
				volume: bar.v ?? 0,
			})) ?? [];

		this.loadBars(bars);
	}

	private onCandle(ev: CandleBarEvent) {
		this.upsertBar({
			sec: ev.sec,
			open: ev.open,
			high: ev.high,
			low: ev.low,
			close: ev.close,
			volume: ev.volume,
		});
	}

	private loadBars(bars: CandleBar[]) {
		this.chart.ohlc.clear();
		this.chart.volume.clear();
		this.chart.openValues.splice(0);
		this.chart.closeValues.splice(0);

		for (const bar of bars) {
			this.appendBar(bar);
		}

		this.refreshChartRanges("force");
	}

	private upsertBar(bar: CandleBar) {
		this.requireBar(bar);

		const index = this.findBarIndex(bar.sec);
		const isNewBar = index < 0;

		if (isNewBar) {
			this.appendBar(bar);
		} else {
			this.writeBar(index, bar);
		}

		this.refreshChartRanges(isNewBar ? "follow" : "none");
	}

	private refreshChartRanges(scrollMode: "force" | "follow" | "none") {
		refreshFinancialPriceAxis(this.chart.yAxis, this.chart.ohlc);

		if (scrollMode === "force") {
			followLatestCandleRange(this.chart.xAxis, this.chart.ohlc, true);
		}

		if (scrollMode === "follow") {
			followLatestCandleRange(this.chart.xAxis, this.chart.ohlc, false);
		}

		this.chart.volumeSeries.invalidateElement();
	}

	private requireBar(bar: CandleBar) {
		const values = [bar.sec, bar.open, bar.high, bar.low, bar.close];

		if (values.every((value) => Number.isFinite(value)) && bar.close > 0) {
			return;
		}

		throw new Error(`invalid candle bar for ${this.symbol}`);
	}

	private findBarIndex(sec: number) {
		const nativeX = this.chart.ohlc.getNativeXValues();

		for (let index = this.chart.ohlc.count() - 1; index >= 0; index--) {
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
		this.requireBar(bar);
		this.chart.ohlc.append(bar.sec, bar.open, bar.high, bar.low, bar.close);
		this.chart.volume.append(bar.sec, this.candleVolume(bar));
		this.chart.openValues.push(bar.open);
		this.chart.closeValues.push(bar.close);
	}

	private writeBar(index: number, bar: CandleBar) {
		this.requireBar(bar);
		this.chart.ohlc.update(index, bar.open, bar.high, bar.low, bar.close);
		this.chart.volume.update(index, this.candleVolume(bar));
		this.chart.openValues[index] = bar.open;
		this.chart.closeValues[index] = bar.close;
	}

	private candleVolume(bar: CandleBar): number {
		if (bar.volume < 0 || !Number.isFinite(bar.volume)) {
			throw new Error(`invalid candle volume ${bar.volume}`);
		}

		return bar.volume;
	}
}
