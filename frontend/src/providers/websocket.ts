import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { appStore } from "#/collections/app";
import {
	type PlaybookBranch,
	parseWalkTrace,
	playbookStore,
} from "#/collections/playbook";
import {
	isSignalDiagnosticReading,
	parseGaugeFrame,
	signalStore,
} from "#/collections/signals";
import { normalizeWireFrame } from "#/components/charts/confidence/gauge-frame";
import { routeWireFrame } from "#/lib/symm/frame-router";
import {
	decisionTreeBranches,
	finiteCount,
	gaugeFramesFromState,
	isRecord,
} from "#/providers/websocket-handlers";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const WIRE_ERROR_LOG_INTERVAL_MS = 5000;

let lastWireErrorAt = 0;

const parseOptionalWalkTrace = (value: unknown) =>
	isRecord(value) ? parseWalkTrace(value) : null;

const applyGaugeFrame = (frame: Record<string, unknown>) => {
	const normalized = normalizeWireFrame(frame);
	const source = typeof normalized.source === "string" ? normalized.source : "";
	const reading = parseGaugeFrame(normalized);

	if (reading !== null && isSignalDiagnosticReading(reading)) {
		signalStore.actions.updateReading(reading);
	}

	if (source !== "") {
		appStore.actions.stashGaugeFrame(source, normalized);
	}

	appStore.state.confidenceHeatmapUpdater?.(normalized);
	appStore.state.surpriseHeatmapUpdater?.(normalized);
};

const applyCandleFrame = (frame: Record<string, unknown>) => {
	const symbol = typeof frame.symbol === "string" ? frame.symbol : "";
	const updater = appStore.state.candleUpdaters[symbol.trim().toUpperCase()];

	updater?.(frame);
};

export const WsFeed = () => {
	const {
		updateOnline,
		updatePlaybookEvaluations,
		updateStoryTicks,
		stashRegimeFrame,
		stashManifoldFrame,
		stashResonanceFrame,
	} = appStore.actions;
	const { updateBranches, updateWalkTrace } = playbookStore.actions;

	useWebSocket(socketUrl, {
		shouldReconnect: () => true,
		onOpen: () => updateOnline(true),
		onClose: () => updateOnline(false),
		onMessage: (event) => {
			try {
				const raw = JSON.parse(event.data) as Record<string, unknown>;

				routeWireFrame(raw);

				switch (raw.type) {
					case "state": {
						const storyTicks = finiteCount(raw.story_ticks);
						const playbookEvaluations = finiteCount(raw.playbook_evaluations);
						const decisionWalk = parseOptionalWalkTrace(raw.decision_walk);
						const walkTrace = parseOptionalWalkTrace(raw.walk);

						if (decisionWalk !== null) {
							updateWalkTrace(decisionWalk);
						}

						if (storyTicks !== null) {
							updateStoryTicks(storyTicks);
						}

						if (playbookEvaluations !== null) {
							updatePlaybookEvaluations(playbookEvaluations);
						}

						if (walkTrace !== null) {
							updateWalkTrace(walkTrace);
						}

						for (const frame of gaugeFramesFromState(raw)) {
							applyGaugeFrame(frame);
						}

						break;
					}
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
							updateBranches(branches as PlaybookBranch[]);
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
					case "resonance_universe":
						stashResonanceFrame(raw);
						break;
					case "prediction":
						appStore.state.predictionUpdater?.(raw);
						break;
					default:
						break;
				}
			} catch (error) {
				const now = Date.now();

				if (now - lastWireErrorAt < WIRE_ERROR_LOG_INTERVAL_MS) {
					return;
				}

				lastWireErrorAt = now;
				console.error("websocket frame parse failed", error);
			}
		},
	});

	return null;
};
