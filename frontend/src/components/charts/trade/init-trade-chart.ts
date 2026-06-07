import type { OhlcDataSeries, SciChartSurface } from "scichart";

import {
	createFinancialChartSurface,
	followLatestCandleRange,
	isViewportFollowingLiveEdge,
	refreshFinancialPriceAxis,
} from "#/components/charts/shared/financial-chart-utils";
import type { OhlcCandle } from "#/components/charts/trade/trade-chart-wire";
import { ensureSciChartWasm } from "#/lib/utils";

export type TTradeChartInitResult = {
	sciChartSurface: SciChartSurface;
	appendBar: (bar: OhlcCandle) => void;
};

const defaultBarStepSec = (ohlc: OhlcDataSeries): number => {
	const barCount = ohlc.count();

	if (barCount <= 1) {
		return 60;
	}

	const nativeX = ohlc.getNativeXValues();
	const lastIndex = barCount - 1;

	return nativeX.get(lastIndex) - nativeX.get(lastIndex - 1);
};

export const initTradeChart = async (
	rootElement: HTMLDivElement,
	symbol: string,
): Promise<TTradeChartInitResult> => {
	await ensureSciChartWasm();

	const chart = await createFinancialChartSurface(rootElement, symbol);
	const {
		sciChartSurface,
		xAxis,
		yAxis,
		ohlc,
		volume,
		openValues,
		closeValues,
	} = chart;

	const appendBar = (bar: OhlcCandle) => {
		const nativeX = ohlc.getNativeXValues();
		const lastIndex = ohlc.count() - 1;
		const priorLastX = lastIndex >= 0 ? nativeX.get(lastIndex) : null;
		const barStep = defaultBarStepSec(ohlc);
		const following =
			priorLastX === null ||
			isViewportFollowingLiveEdge(xAxis.visibleRange, priorLastX, barStep);

		if (priorLastX === bar.sec) {
			ohlc.update(lastIndex, bar.open, bar.high, bar.low, bar.close);
			volume.update(lastIndex, bar.volume);
			openValues[lastIndex] = bar.open;
			closeValues[lastIndex] = bar.close;
		} else {
			ohlc.append(bar.sec, bar.open, bar.high, bar.low, bar.close);
			volume.append(bar.sec, bar.volume);
			openValues.push(bar.open);
			closeValues.push(bar.close);
		}

		refreshFinancialPriceAxis(yAxis, ohlc);
		sciChartSurface.invalidateElement();

		if (!following) {
			return;
		}

		followLatestCandleRange(
			xAxis,
			ohlc,
			priorLastX === null ? "initial" : "live",
		);
	};

	return {
		sciChartSurface,
		appendBar,
	};
};
