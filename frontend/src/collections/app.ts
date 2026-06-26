import { createStore } from "@tanstack/react-store";

const replayDashboardFrame = (
  updater: ((frame: Record<string, unknown>) => void) | null,
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
    storySessionStartedAt: null as number | null,
    enginePhase: "",
    chartThrottled: false,
    lastRegimeFrame: null as Record<string, unknown> | null,
    lastFluidFrame: null as Record<string, unknown> | null,
    lastManifoldFrame: null as Record<string, unknown> | null,
    lastResonanceFrame: null as Record<string, unknown> | null,
    lastGaugeFrames: {} as Record<string, Record<string, unknown>>,
    candleUpdaters: {} as Record<string, (frame: Record<string, unknown>) => void>,
    gaugeUpdaters: {} as Record<string, (frame: Record<string, unknown>) => void>,
    regimeUpdater: null as ((frame: Record<string, unknown>) => void) | null,
    fluidUpdater: null as ((frame: Record<string, unknown>) => void) | null,
    manifoldUpdater: null as ((frame: Record<string, unknown>) => void) | null,
    resonanceUpdater: null as ((frame: Record<string, unknown>) => void) | null,
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
        storySessionStartedAt:
          prev.storySessionStartedAt ?? (storyTicks > 0 ? Date.now() : null),
      })),
    updateEnginePhase: (enginePhase: string) =>
      setState((prev) => ({
        ...prev,
        enginePhase: enginePhase.trim(),
      })),
    updateChartThrottled: (chartThrottled: boolean) =>
      setState((prev) => ({
        ...prev,
        chartThrottled: chartThrottled,
      })),
    updateCandleUpdater: (
      symbol: string,
      candleUpdater: ((frame: Record<string, unknown>) => void) | null,
    ) =>
      setState((prev) => {
        const candleUpdaters = { ...prev.candleUpdaters };
        const normalized = symbol.trim().toUpperCase();

        if (normalized === "") {
          return prev;
        }

        if (candleUpdater === null) {
          delete candleUpdaters[normalized];

          return {
            ...prev,
            candleUpdaters: candleUpdaters,
          };
        }

        candleUpdaters[normalized] = candleUpdater;

        return {
          ...prev,
          candleUpdaters: candleUpdaters,
        };
      }),
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
          const lastFrame = prev.lastGaugeFrames[source];

          if (lastFrame !== undefined) {
            gaugeUpdater(lastFrame);
          }
        }

        return {
          ...prev,
          gaugeUpdaters: gaugeUpdaters,
        };
      }),
    stashGaugeFrame: (source: string, frame: Record<string, unknown>) =>
      setState((prev) => {
        if (source === "") {
          return prev;
        }

        const lastGaugeFrames = {
          ...prev.lastGaugeFrames,
          [source]: frame,
        };

        prev.gaugeUpdaters[source]?.(frame);

        return {
          ...prev,
          lastGaugeFrames: lastGaugeFrames,
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
    stashFluidFrame: (frame: Record<string, unknown>) =>
      setState((prev) => {
        replayDashboardFrame(prev.fluidUpdater, frame);

        return {
          ...prev,
          lastFluidFrame: frame,
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
    updateRegimeUpdater: (
      regimeUpdater: ((frame: Record<string, unknown>) => void) | null,
    ) =>
      setState((prev) => {
        replayDashboardFrame(regimeUpdater, prev.lastRegimeFrame);

        return {
          ...prev,
          regimeUpdater: regimeUpdater,
        };
      }),
    updateFluidUpdater: (
      fluidUpdater: ((frame: Record<string, unknown>) => void) | null,
    ) =>
      setState((prev) => {
        replayDashboardFrame(fluidUpdater, prev.lastFluidFrame);

        return {
          ...prev,
          fluidUpdater: fluidUpdater,
        };
      }),
    updateManifoldUpdater: (
      manifoldUpdater: ((frame: Record<string, unknown>) => void) | null,
    ) =>
      setState((prev) => {
        replayDashboardFrame(manifoldUpdater, prev.lastManifoldFrame);

        return {
          ...prev,
          manifoldUpdater: manifoldUpdater,
        };
      }),
    stashResonanceFrame: (frame: Record<string, unknown>) =>
      setState((prev) => {
        replayDashboardFrame(prev.resonanceUpdater, frame);

        return {
          ...prev,
          lastResonanceFrame: frame,
        };
      }),
    updateResonanceUpdater: (
      resonanceUpdater: ((frame: Record<string, unknown>) => void) | null,
    ) =>
      setState((prev) => {
        replayDashboardFrame(resonanceUpdater, prev.lastResonanceFrame);

        return {
          ...prev,
          resonanceUpdater: resonanceUpdater,
        };
      }),
    stashPredictionFrame: (frame: Record<string, unknown>) =>
      setState((prev) => {
        prev.predictionUpdater?.(frame);
        return prev;
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
