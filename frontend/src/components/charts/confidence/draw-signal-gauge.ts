import {
	Rect,
	SciChartPolarSubSurface,
	SciChartPolarSurface,
	SciChartSubSurface,
	Thickness,
} from "scichart";

import { createConfidenceSubChart } from "#/components/charts/confidence/confidence-subchart";
import {
	gaugeConfidenceReading,
	gaugeSurpriseReading,
} from "#/components/charts/confidence/gauge-frame";
import { createSurpriseSubChart } from "#/components/charts/confidence/surprise-subchart";
import { appTheme } from "#/components/charts/shared/theme";
import { ensureSciChartWasm } from "#/lib/utils";

const CONFIDENCE_SUBCHART_RECT = new Rect(0, 0, 1, 0.86);
const SURPRISE_SUBCHART_RECT = new Rect(0, 0.86, 1, 0.14);

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

export const drawSignalGauge = async (rootElement: string | HTMLDivElement) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartPolarSurface.create(
		rootElement,
		{
			padding: new Thickness(0, 0, 0, 0),
			background: appTheme.Background,
			freezeWhenOutOfView: true,
		},
	);

	const confidenceSubChart = SciChartPolarSubSurface.createSubSurface(
		sciChartSurface,
		{
			position: CONFIDENCE_SUBCHART_RECT,
			padding: new Thickness(0, 0, 0, 0),
			background: appTheme.Background,
		},
	);

	const surpriseSubChart = SciChartSubSurface.createSubSurface(
		sciChartSurface,
		{
			position: SURPRISE_SUBCHART_RECT,
			padding: new Thickness(0, 0, 0, 0),
			background: appTheme.Background,
		},
	);

	const confidenceControls = createConfidenceSubChart(confidenceSubChart);
	const surpriseControls = createSurpriseSubChart(surpriseSubChart);

	const addData = (frame: Record<string, unknown>) => {
		const confidence = gaugeConfidenceReading(frame) ?? 0;
		const surprise = gaugeSurpriseReading(frame) ?? 0;

		const thresholdReading = finiteNumber(frame.surprise_threshold);
		const threshold =
			thresholdReading !== null ? Math.max(0.1, thresholdReading) : 2;

		confidenceControls.update(confidence, false);
		surpriseControls.update(surprise, threshold * 3, threshold);
	};

	return {
		sciChartSurface,
		wasmContext,
		addData,
	};
};
