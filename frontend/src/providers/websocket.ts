import * as capnp from "capnp-ts";
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
import { Artifact } from "#/lib/capnp/artifact.capnp";
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
					let buffer: ArrayBuffer | null = null;

					if (event.data instanceof ArrayBuffer) {
						buffer = event.data;
					}

					if (event.data instanceof Blob) {
						buffer = await event.data.arrayBuffer();
					}

					if (buffer === null) {
						return;
					}

					const message = new capnp.Message(buffer, true);
					const artifact = message.getRoot(Artifact);

					const attributesText = artifact.hasAttributes()
						? new TextDecoder()
								.decode(artifact.getAttributes().toUint8Array())
								.trim()
						: "";
					const attributesJSON =
						attributesText === "" ? {} : JSON.parse(attributesText);

					let payloadJSON: Record<string, unknown> = {};

					if (artifact.hasEncryptedPayload() && artifact.hasEncryptedKey()) {
						const encryptedKey = new Uint8Array(
							artifact.getEncryptedKey().toUint8Array(),
						);
						const encryptedPayload = new Uint8Array(
							artifact.getEncryptedPayload().toUint8Array(),
						);

						if (encryptedKey.length === 32 && encryptedPayload.length > 12) {
							const cryptoKey = await crypto.subtle.importKey(
								"raw",
								encryptedKey,
								"AES-GCM",
								false,
								["decrypt"],
							);
							const plaintext = await crypto.subtle.decrypt(
								{ name: "AES-GCM", iv: encryptedPayload.slice(0, 12) },
								cryptoKey,
								encryptedPayload.slice(12),
							);
							const payloadText = new TextDecoder().decode(plaintext).trim();

							if (payloadText !== "") {
								payloadJSON = JSON.parse(payloadText) as Record<
									string,
									unknown
								>;
							}
						}
					}

					const frame = {
						...attributesJSON,
						...payloadJSON,
						role: artifact.getRole(),
						scope: artifact.getScope(),
						origin: artifact.getOrigin(),
						destination: artifact.getDestination(),
					};

					console.log("frame", frame);

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
