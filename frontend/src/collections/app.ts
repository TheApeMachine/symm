import { createStore } from "@tanstack/react-store";

type DashboardFrameUpdater = (frame: Record<string, unknown>) => void;

const replayDashboardFrame = (
	updater: DashboardFrameUpdater | null,
	frame: Record<string, unknown> | null,
) => {
	if (updater !== null && frame !== null) {
		updater(frame);
	}
};

export const appStore = createStore(
	{
		online: false,
		showPositions: false,
		playbookEvaluations: 0,
		storyTicks: 0,
		lastRegimeFrame: null as Record<string, unknown> | null,
		lastManifoldFrame: null as Record<string, unknown> | null,
		candleUpdater: null as DashboardFrameUpdater | null,
		gaugeUpdaters: {} as Record<string, DashboardFrameUpdater>,
		regimeUpdater: null as DashboardFrameUpdater | null,
		fluidUpdater: null as DashboardFrameUpdater | null,
		manifoldUpdater: null as DashboardFrameUpdater | null,
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
		stashRegimeFrame: (frame: Record<string, unknown>) =>
			setState((prev) => {
				replayDashboardFrame(prev.regimeUpdater, frame);

				return {
					...prev,
					lastRegimeFrame: frame,
				};
			}),
		stashManifoldFrame: (frame: Record<string, unknown>) =>
			setState((prev) => {
				replayDashboardFrame(prev.manifoldUpdater, frame);

				return {
					...prev,
					lastManifoldFrame: frame,
				};
			}),
		updateRegimeUpdater: (regimeUpdater: DashboardFrameUpdater | null) =>
			setState((prev) => {
				replayDashboardFrame(regimeUpdater, prev.lastRegimeFrame);

				return {
					...prev,
					regimeUpdater: regimeUpdater,
				};
			}),
		updateFluidUpdater: (fluidUpdater: DashboardFrameUpdater | null) =>
			setState((prev) => ({
				...prev,
				fluidUpdater: fluidUpdater,
			})),
		updateManifoldUpdater: (manifoldUpdater: DashboardFrameUpdater | null) =>
			setState((prev) => {
				replayDashboardFrame(manifoldUpdater, prev.lastManifoldFrame);

				return {
					...prev,
					manifoldUpdater: manifoldUpdater,
				};
			}),
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
