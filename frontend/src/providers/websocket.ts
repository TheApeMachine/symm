import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import { appStore } from "#/collections/app";
import { cognitiveStore, parseCognitiveFrame } from "#/collections/cognitive";
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
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";
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

const wireBufferFromMessage = async (
	data: MessageEvent["data"],
): Promise<ArrayBuffer | null> => {
	if (data instanceof ArrayBuffer) {
		return data;
	}

	if (data instanceof Blob) {
		return data.arrayBuffer();
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
		stashResonanceFrame,
	} = appStore.actions;

	useWebSocket(socketUrl, {
		shouldReconnect: () => true,
		onOpen: () => updateOnline(true),
		onClose: () => updateOnline(false),
		onMessage: (event) => {
			void (async () => {
				try {
					const buffer = await wireBufferFromMessage(event.data);

					if (buffer === null) {
						return;
					}

					const frame = await decodePackedArtifactWire(buffer);

					if (frame === null) {
						return;
					}

					if (frame.role === "measurement") {
						applyGaugeFrame(frame);
					} else {
						routeWireFrame(frame);
					}
				} catch (error) {
					const now = Date.now();

					if (now - lastWireErrorAt < WIRE_ERROR_LOG_INTERVAL_MS) {
						return;
					}

					lastWireErrorAt = now;
					console.error("websocket frame parse failed", error);
				}
			})();
		},
	});

	return null;
};
