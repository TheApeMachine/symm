import { memo } from "react";
import {
	EAutoRange,
	FastLineRenderableSeries,
	NumberRange,
	NumericAxis,
	SciChartSurface,
	XyDataSeries,
} from "scichart";
import { SciChartReact } from "scichart-react";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { appTheme } from "#/components/charts/shared/theme";
import { ensureSciChartWasm } from "#/lib/utils";

const HISTORY = 120;

const initSparkline = async (rootElement: string | HTMLDivElement) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartSurface.create(
		rootElement,
		{ freezeWhenOutOfView: true },
	);

	sciChartSurface.background = "transparent";

	const xValues = Array.from({ length: HISTORY }, (_, index) => index);
	const yValues = Array.from({ length: HISTORY }, () => 0);

	sciChartSurface.xAxes.add(
		new NumericAxis(wasmContext, {
			isVisible: false,
			autoRange: EAutoRange.Never,
			visibleRange: new NumberRange(0, HISTORY - 1),
		}),
	);

	sciChartSurface.yAxes.add(
		new NumericAxis(wasmContext, {
			isVisible: false,
			autoRange: EAutoRange.Never,
			visibleRange: new NumberRange(0, 1),
		}),
	);

	const dataSeries = new XyDataSeries(wasmContext, { xValues, yValues });

	sciChartSurface.renderableSeries.add(
		new FastLineRenderableSeries(wasmContext, {
			dataSeries,
			stroke: appTheme.VividSkyBlue,
			strokeThickness: 2,
		}),
	);

	const addData = (confidence: number) => {
		for (let index = 0; index < HISTORY - 1; index += 1) {
			yValues[index] = yValues[index + 1];
		}

		yValues[HISTORY - 1] = Math.min(1, Math.max(0, confidence));
		dataSeries.clear();
		dataSeries.appendRange(xValues, yValues);
		sciChartSurface.invalidateElement();
	};

	return { sciChartSurface, wasmContext, addData };
};

const subscribeSparkline = (
	origin: string,
	addData: (confidence: number) => void,
) => {
	let lastConfidence = Number.NaN;

	const push = () => {
		const focusSymbol = terminalStore.state.focusSymbol;
		const frame = measurementsStore.state.readings[origin]?.[focusSymbol];
		const output = (frame?.output ?? {}) as Record<string, unknown>;
		const confidence = (output.confidence as number) ?? 0;

		if (Number.isNaN(lastConfidence) || confidence !== lastConfidence) {
			lastConfidence = confidence;
			addData(confidence);
		}
	};

	push();

	const subscription = measurementsStore.subscribe(push);
	const focusSubscription = terminalStore.subscribe(push);

	return () => {
		subscription.unsubscribe();
		focusSubscription.unsubscribe();
	};
};

export const SignalSparkline = memo(function SignalSparkline({
	origin,
}: {
	origin: string;
}) {
	return (
		<SciChartReact
			initChart={initSparkline}
			onInit={(result) => subscribeSparkline(origin, result.addData)}
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
		/>
	);
});
