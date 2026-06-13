import type { OhlcDataSeries } from "scichart";
import type { StopLossTakeProfitAnnotation } from "scichart-financial-tools";
import {
	createFinancialChartSurface,
	followLatestCandleRange,
	refreshFinancialPriceAxis,
	type FinancialChartContext,
} from "#/components/charts/shared/financial-chart-utils";
import {
	type StopLossOverlayInput,
	syncStopLossAnnotation,
} from "#/components/charts/trade/stop-loss-annotation";

export type CandleFrame = {
	symbol: string;
	sec: number;
	open: number;
	high: number;
	low: number;
	close: number;
	volume: number;
};

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

export const normalizeSymbol = (symbol: string): string =>
	symbol.trim().toUpperCase();

export const parseCandleFrame = (
	frame: Record<string, unknown>,
): CandleFrame | null => {
	const symbol = typeof frame.symbol === "string" ? normalizeSymbol(frame.symbol) : "";
	const sec = finiteNumber(frame.sec);
	const open = finiteNumber(frame.open);
	const high = finiteNumber(frame.high);
	const low = finiteNumber(frame.low);
	const close = finiteNumber(frame.close);
	const volume = finiteNumber(frame.volume) ?? 0;

	if (
		symbol === "" ||
		sec === null ||
		open === null ||
		high === null ||
		low === null ||
		close === null
	) {
		return null;
	}

	return {
		symbol,
		sec,
		open,
		high,
		low,
		close,
		volume,
	};
};

export const latestCandleTime = (ohlc: OhlcDataSeries): number | null => {
	const count = ohlc.count();

	if (count <= 0) {
		return null;
	}

	return ohlc.getNativeXValues().get(count - 1);
};

export const applyCandleFrame = (
	chart: FinancialChartContext,
	candle: CandleFrame,
) => {
	const count = chart.ohlc.count();
	const latest = latestCandleTime(chart.ohlc);

	if (latest !== null && candle.sec < latest) {
		return;
	}

	if (latest !== null && candle.sec === latest) {
		chart.ohlc.update(
			count - 1,
			candle.open,
			candle.high,
			candle.low,
			candle.close,
		);
		chart.volume.update(count - 1, candle.volume);
		refreshFinancialPriceAxis(chart.yAxis, chart.ohlc);
		return;
	}

	chart.ohlc.append(
		candle.sec,
		candle.open,
		candle.high,
		candle.low,
		candle.close,
	);
	chart.volume.append(candle.sec, candle.volume);
	followLatestCandleRange(chart.xAxis, chart.ohlc, count === 0 ? "initial" : "live");
	refreshFinancialPriceAxis(chart.yAxis, chart.ohlc);
};

const chartElement = (rootElement: string | HTMLDivElement): HTMLDivElement => {
	if (typeof rootElement !== "string") {
		return rootElement;
	}

	const element = document.getElementById(rootElement);

	if (element instanceof HTMLDivElement) {
		return element;
	}

	throw new Error("trade chart root element not found");
};

export const initTradeChart = async (
	rootElement: string | HTMLDivElement,
	symbol: string,
) => {
	const normalizedSymbol = normalizeSymbol(symbol);
	const chart = await createFinancialChartSurface(
		chartElement(rootElement),
		normalizedSymbol,
	);

	let stopAnnotation: StopLossTakeProfitAnnotation | null = null;
	let stopPrevious: {
		overlay: StopLossOverlayInput;
		x0: number;
		x1: number;
	} | null = null;
	let stopOverlay: StopLossOverlayInput | null = null;

	const refreshStopLoss = () => {
		const synced = syncStopLossAnnotation(
			chart,
			stopAnnotation,
			stopOverlay,
			stopPrevious,
		);

		stopAnnotation = synced.annotation;
		stopPrevious = synced.previous;
	};

	const addData = (frame: Record<string, unknown>) => {
		const candle = parseCandleFrame(frame);

		if (candle === null || candle.symbol !== normalizedSymbol) {
			return;
		}

		applyCandleFrame(chart, candle);
		refreshStopLoss();
	};

	const updateStopLoss = (overlay: StopLossOverlayInput | null) => {
		stopOverlay = overlay;
		refreshStopLoss();
	};

	return {
		...chart,
		addData,
		updateStopLoss,
	};
};
