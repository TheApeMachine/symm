import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { appStore } from "#/collections/app";
import {
  type PlaybookBranch,
  parseWalkTrace,
  playbookStore,
} from "#/collections/playbook";
import { applyBalanceFrame } from "#/collections/positions";
import { parseGaugeFrame, signalStore } from "#/collections/signals";
import { normalizeWireFrame } from "#/components/charts/confidence/gauge-frame";

const socketUrl =
  import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const finiteCount = (value: unknown): number | null => {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    return null;
  }

  return Math.floor(value);
};

const isPlaybookBranch = (value: unknown): value is PlaybookBranch => {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const branch = value as PlaybookBranch;

  if (Array.isArray(branch.branches)) {
    return branch.branches.every(isPlaybookBranch);
  }

  return true;
};

const gaugeFramesFromState = (
  raw: Record<string, unknown>,
): Record<string, unknown>[] => {
  const gaugeReadings = raw.gauge_readings;

  if (Array.isArray(gaugeReadings)) {
    return gaugeReadings.filter(
      (frame): frame is Record<string, unknown> =>
        typeof frame === "object" && frame !== null,
    );
  }

  const measurements = raw.measurements;

  if (!Array.isArray(measurements)) {
    return [];
  }

  return measurements.filter(
    (frame): frame is Record<string, unknown> =>
      typeof frame === "object" && frame !== null,
  );
};

const applyGaugeFrame = (frame: Record<string, unknown>) => {
  const normalized = normalizeWireFrame(frame);
  const source = typeof normalized.source === "string" ? normalized.source : "";
  const reading = parseGaugeFrame(normalized);

  if (reading !== null) {
    signalStore.actions.updateReading(reading);
  }

  if (source !== "") {
    appStore.state.gaugeUpdaters[source]?.(normalized);
  }

  appStore.state.confidenceHeatmapUpdater?.(normalized);
  appStore.state.surpriseHeatmapUpdater?.(normalized);
};

const applyCandleFrame = (frame: Record<string, unknown>) => {
  const symbol = typeof frame.symbol === "string" ? frame.symbol : "";
  const updater = appStore.state.candleUpdaters[symbol.trim().toUpperCase()];

  updater?.(frame);
};

const decisionTreeBranches = (
  raw: Record<string, unknown>,
): PlaybookBranch[] | null => {
  const topLevel = raw.branches;

  if (Array.isArray(topLevel) && topLevel.every(isPlaybookBranch)) {
    return topLevel;
  }

  const nested = raw.value;

  if (typeof nested === "object" && nested !== null) {
    const nestedBranches = (nested as Record<string, unknown>).branches;

    if (
      Array.isArray(nestedBranches) &&
      nestedBranches.every(isPlaybookBranch)
    ) {
      return nestedBranches;
    }
  }

  return null;
};

export const WsFeed = () => {
  const {
    updateOnline,
    updatePlaybookEvaluations,
    updateStoryTicks,
    stashRegimeFrame,
    stashManifoldFrame,
  } = appStore.actions;
  const { updateBranches, updateWalkTrace } = playbookStore.actions;

  useWebSocket(socketUrl, {
    shouldReconnect: () => true,
    onOpen: () => updateOnline(true),
    onClose: () => updateOnline(false),
    onMessage: (event) => {
      try {
        const raw = JSON.parse(event.data) as Record<string, unknown>;

        switch (raw.type) {
          case "state": {
            const walkTrace = raw.walk as Record<string, unknown>;
            const storyTicks = finiteCount(raw.story_ticks);
            const playbookEvaluations = finiteCount(raw.playbook_evaluations);
            const decisionWalk = raw.decision_walk as Record<string, unknown>;

            if (decisionWalk !== null) {
              updateWalkTrace(parseWalkTrace(decisionWalk));
            }

            if (storyTicks !== null) {
              updateStoryTicks(storyTicks);
            }

            if (playbookEvaluations !== null) {
              updatePlaybookEvaluations(playbookEvaluations);
            }

            if (walkTrace !== null) {
              updateWalkTrace(parseWalkTrace(walkTrace));
            }

            for (const frame of gaugeFramesFromState(raw)) {
              applyGaugeFrame(frame);
            }

            break;
          }
          case "balances":
            applyBalanceFrame(raw);
            break;
          case "story": {
            const storyTicks = finiteCount(raw.story_ticks);

            if (storyTicks !== null) {
              updateStoryTicks(storyTicks);
            }

            const playbookEvaluations = finiteCount(raw.playbook_evaluations);

            if (playbookEvaluations !== null) {
              updatePlaybookEvaluations(playbookEvaluations);
            }

            break;
          }
          case "decision_tree": {
            const branches = decisionTreeBranches(raw);

            if (branches !== null) {
              updateBranches(branches);
            }

            break;
          }
          case "decision_walk": {
            const walkTrace = parseWalkTrace(raw);

            if (walkTrace !== null) {
              updateWalkTrace(walkTrace);
            }

            break;
          }
          case "ohlc":
            applyCandleFrame(raw);
            break;
          case "gauge":
            applyGaugeFrame(raw);
            break;
          case "regime":
            stashRegimeFrame(raw);
            break;
          case "fluid":
            appStore.state.fluidUpdater?.(raw);
            break;
          case "manifold":
            stashManifoldFrame(raw);
            break;
          case "prediction":
            appStore.state.predictionUpdater?.(raw);
            break;
          default:
            break;
        }
      } catch (error) {
        console.error("websocket frame parse failed", error, event.data);
      }
    },
  });

  return null;
};
