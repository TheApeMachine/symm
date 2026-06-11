import {
	Rect,
	SciChartPolarSubSurface,
	SciChartPolarSurface,
	SciChartSubSurface,
	Thickness,
} from "scichart";

import { createConfidenceSubChart } from "#/components/charts/confidence/confidence-subchart";
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
		const calibrating = frame.calibrating === true;
		const samples = finiteNumber(frame.samples) ?? 0;
		const minSamples = finiteNumber(frame.min_samples) ?? 0;
		const warmupProgress =
			calibrating && minSamples > 0
				? Math.min(1, Math.max(0, samples / minSamples))
				: null;

		const confidence =
			warmupProgress !== null
				? warmupProgress
				: (finiteNumber(frame.confidence) ?? 0);

		const surpriseReading = frame.surprise ?? frame.snr;
		const measuredSurprise =
			typeof surpriseReading === "number" && Number.isFinite(surpriseReading)
				? Math.max(0, surpriseReading)
				: 0;
		const surprise =
			warmupProgress !== null ? warmupProgress : measuredSurprise;

		const thresholdReading = finiteNumber(frame.surprise_threshold);
		const threshold =
			thresholdReading !== null ? Math.max(0.1, thresholdReading) : 2;

		confidenceControls.update(confidence, warmupProgress !== null);
		surpriseControls.update(surprise, threshold * 3, threshold);
	};

	return {
		sciChartSurface,
		wasmContext,
		addData,
	};
};
