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

export type SignalGaugeControls = {
	updateConfidence: (confidence: number) => void;
	updateSurprise: (
		surprise: number,
		scaleMax: number,
		threshold: number,
	) => void;
	dispose: () => void;
};

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

	return {
		sciChartSurface,
		wasmContext,
		confidenceSubChart,
		surpriseSubChart,
		controls: {
			updateConfidence: confidenceControls.update,
			updateSurprise: surpriseControls.update,
			dispose() {
				// SciChartReact deletes the parent surface on unmount.
			},
		} satisfies SignalGaugeControls,
	};
};
