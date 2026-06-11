import { createStore } from "@tanstack/react-store";

export const appStore = createStore(
	{
		online: false,
		showPositions: false,
		playbookEvaluations: 0,
		storyTicks: 0,
		candleUpdater: null as ((frame: Record<string, unknown>) => void) | null,
		gaugeUpdaters: {} as Record<
			string,
			(frame: Record<string, unknown>) => void
		>,
		regimeUpdater: null as ((frame: Record<string, unknown>) => void) | null,
		fluidUpdater: null as ((frame: Record<string, unknown>) => void) | null,
		manifoldUpdater: null as ((frame: Record<string, unknown>) => void) | null,
		predictionUpdater: null as
			| ((frame: Record<string, unknown>) => void)
			| null,
		confidenceHeatmapUpdater: null as
			| ((frame: Record<string, unknown>) => void)
			| null,
		surpriseHeatmapUpdater: null as
			| ((frame: Record<string, unknown>) => void)
			| null,
	},
	({ setState }) => ({
		updateOnline: (online: boolean) =>
			setState((prev) => ({
				...prev,
				online: online,
			})),
		updateShowPositions: (showPositions: boolean) =>
			setState((prev) => ({
				...prev,
				showPositions: showPositions,
			})),
		updatePlaybookEvaluations: (playbookEvaluations: number) =>
			setState((prev) => ({
				...prev,
				playbookEvaluations: playbookEvaluations,
			})),
		updateStoryTicks: (storyTicks: number) =>
			setState((prev) => ({
				...prev,
				storyTicks: storyTicks,
			})),
		updateCandleUpdater: (
			candleUpdater: ((frame: Record<string, unknown>) => void) | null,
		) =>
			setState((prev) => ({
				...prev,
				candleUpdater: candleUpdater,
			})),
		updateGaugeUpdater: (
			source: string,
			gaugeUpdater: ((frame: Record<string, unknown>) => void) | null,
		) =>
			setState((prev) => {
				const gaugeUpdaters = { ...prev.gaugeUpdaters };

				if (gaugeUpdater === null) {
					delete gaugeUpdaters[source];
				} else {
					gaugeUpdaters[source] = gaugeUpdater;
				}

				return {
					...prev,
					gaugeUpdaters: gaugeUpdaters,
				};
			}),
		updateRegimeUpdater: (
			regimeUpdater: ((frame: Record<string, unknown>) => void) | null,
		) =>
			setState((prev) => ({
				...prev,
				regimeUpdater: regimeUpdater,
			})),
		updateFluidUpdater: (
			fluidUpdater: ((frame: Record<string, unknown>) => void) | null,
		) =>
			setState((prev) => ({
				...prev,
				fluidUpdater: fluidUpdater,
			})),
		updateManifoldUpdater: (
			manifoldUpdater: ((frame: Record<string, unknown>) => void) | null,
		) =>
			setState((prev) => ({
				...prev,
				manifoldUpdater: manifoldUpdater,
			})),
		updatePredictionUpdater: (
			predictionUpdater: ((frame: Record<string, unknown>) => void) | null,
		) =>
			setState((prev) => ({
				...prev,
				predictionUpdater: predictionUpdater,
			})),
		updateConfidenceHeatmapUpdater: (
			confidenceHeatmapUpdater:
				| ((frame: Record<string, unknown>) => void)
				| null,
		) =>
			setState((prev) => ({
				...prev,
				confidenceHeatmapUpdater: confidenceHeatmapUpdater,
			})),
		updateSurpriseHeatmapUpdater: (
			surpriseHeatmapUpdater: ((frame: Record<string, unknown>) => void) | null,
		) =>
			setState((prev) => ({
				...prev,
				surpriseHeatmapUpdater: surpriseHeatmapUpdater,
			})),
	}),
);
