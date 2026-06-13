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
import { signalStore } from "#/collections/signals";
import { appTheme } from "#/components/charts/shared/theme";
import { ensureSciChartWasm } from "#/lib/utils";

const HISTORY = 120;

/*
A compact rolling confidence sparkline driven by the same per-source gauge feed
the dashboard gauges use. Presentation only — it registers against the existing
appStore.gaugeUpdaters[source] hook and replays incoming frames.
*/
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

/*
Drive the sparkline from signalStore — the same readings the panels show. We
track the per-source updatedAt so we only push genuinely new frames.
*/
const subscribeSparkline = (
	source: string,
	addData: (confidence: number) => void,
) => {
	let lastUpdatedAt = 0;

	const push = () => {
		const reading = signalStore.state.readings[source];

		if (reading === undefined || reading.updatedAt === lastUpdatedAt) {
			return;
		}

		lastUpdatedAt = reading.updatedAt;
		addData(reading.confidence);
	};

	push();

	const subscription = signalStore.subscribe(push);

	return () => {
		subscription.unsubscribe();
	};
};

export const SignalSparkline = memo(function SignalSparkline({
	source,
}: {
	source: string;
}) {
	return (
		<SciChartReact
			initChart={initSparkline}
			onInit={(result) => subscribeSparkline(source, result.addData)}
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
		/>
	);
});
