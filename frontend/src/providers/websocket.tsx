import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	addMeasurement,
	appStore,
	causalStore,
	categoryStore,
	cognitionStore,
	errorStore,
	focusStore,
	onlineStore,
	opportunityStore,
	resonanceArtifactStore,
	resonanceStore,
	tickStore,
} from "#/collections/app";
import { topologyStore } from "#/collections/topology";

import { EnvelopeBoundaryStamp } from "#/providers/telemetry/telemetry/envelope-boundary-stamp";
import { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";
import { ResonanceFrame } from "#/providers/telemetry/telemetry/resonance-frame";

// Tags a ResonanceFrame buffer on the wire (see ui.Hub.encodeResonanceFrame)
// so it can be told apart from an EnvelopeState buffer, which carries no
// identifier of its own, on the same /ws connection.
const RESONANCE_FRAME_IDENTIFIER = "RESO";

let globalWsWorker: Worker | null = null;

export const getWsWorker = () => globalWsWorker;

export const sendBacktestAction = (
	action: "play" | "pause" | "seek" | "select" | "hindsight",
	at?: string,
	captureId?: number,
) => {
	globalWsWorker?.postMessage({
		type: "BACKTEST",
		action,
		at,
		captureId,
	});
};

export const publishBacktestCommand = sendBacktestAction;

export const sendPositionExit = (symbol: string) => {
	globalWsWorker?.postMessage({
		type: "POSITION_EXIT",
		symbol,
	});
};

const defaultWsUrl = () => {
	if (typeof window === "undefined") return "ws://127.0.0.1:8765/ws";
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	const host =
		!window.location.hostname || window.location.hostname === "localhost"
			? "127.0.0.1"
			: window.location.hostname;
	return `${protocol}//${host}:8765/ws`;
};

/*
Dispatches one decoded EnvelopeState into the per-field stores it actually
carries. EnvelopeState is a single envelope's state (one symbol, one point in
time), not a batch of typed frames, so there is no frame-type union to switch
on here — every populated field is pushed to its own store directly.
*/
function dispatchEnvelopeState(state: EnvelopeState) {
	const symbol = state.key() ?? "";

	const measurement = state.correlation();
	if (measurement) addMeasurement("correlation", measurement);

	const leadLag = state.leadLag();
	if (leadLag) addMeasurement("leadlag", leadLag);

	const liquidity = state.liquidity();
	if (liquidity) addMeasurement("liquidity", liquidity);

	const sentiment = state.sentiment();
	if (sentiment) addMeasurement("sentiment", sentiment);

	const cvd = state.cvd();
	if (cvd) addMeasurement("cvd", cvd);

	const depthFlow = state.depthFlow();
	if (depthFlow) addMeasurement("depthflow", depthFlow);

	const morphology = state.morphology();
	if (morphology) addMeasurement("morphology", morphology);

	const hawkes = state.hawkes();
	if (hawkes) addMeasurement("hawkes", hawkes);

	const pumpDump = state.pumpDump();
	if (pumpDump) addMeasurement("pumpdump", pumpDump);

	const toxicity = state.toxicity();
	if (toxicity) addMeasurement("toxicity", toxicity);

	const derivatives = state.derivatives();
	if (derivatives) addMeasurement("derivatives", derivatives);

	if (symbol) {
		const tickerData = state.tickerData();
		if (tickerData) tickStore.actions.add(tickerData);
	}

	const resonance = state.resonance();
	if (resonance) resonanceArtifactStore.actions.add(resonance);

	const cognition = state.cognition();
	if (cognition) cognitionStore.actions.add(cognition);

	const causalOutput = state.causalOutput();
	if (causalOutput) causalStore.actions.add(causalOutput);

	for (let i = 0; i < state.categoriesLength(); i++) {
		const category = state.categories(i);
		if (category) categoryStore.actions.add(category);
	}

	for (let i = 0; i < state.opportunitiesLength(); i++) {
		const opportunity = state.opportunities(i);
		if (opportunity) opportunityStore.actions.add(opportunity);
	}

	const boundaryCount = state.boundariesLength();

	if (boundaryCount > 0) {
		const stamps: EnvelopeBoundaryStamp[] = [];

		for (let i = 0; i < boundaryCount; i++) {
			// A fresh view per row: topologyStore.ingest reads the whole array at
			// once, so a shared view object would have every entry alias the last
			// row read (the same aliasing hazard as the ring-buffered stores).
			const stamp = state.boundaries(i, new EnvelopeBoundaryStamp());
			if (stamp) stamps.push(stamp);
		}

		topologyStore.actions.ingest(stamps);
	}
}

export const WsFeed = () => {
	useEffect(() => {
		const wsWorker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
			type: "module",
		});
		globalWsWorker = wsWorker;

		const wsUrl = import.meta.env.VITE_SYMM_WS_URL || defaultWsUrl();

		wsWorker.addEventListener("message", (event: MessageEvent) => {
			const data = event.data;
			if (!data) return;

			if (data.type === "STATUS") {
				onlineStore.setState(() => data.status);
				appStore.actions.updateOnline(data.status === "ONLINE");
				if (data.status === "ONLINE") {
					wsWorker.postMessage({ type: "FOCUS", symbol: focusStore.state });
				}
				return;
			}

			if (data.type === "ERROR") {
				errorStore.setState(() => new Error(data.error));
				return;
			}

			if (data.type === "BATCH" && data.buffer instanceof ArrayBuffer) {
				try {
					const buffer = new flatbuffers.ByteBuffer(
						new Uint8Array(data.buffer),
					);

					if (buffer.__has_identifier(RESONANCE_FRAME_IDENTIFIER)) {
						const frame = ResonanceFrame.getRootAsResonanceFrame(buffer);
						storeBatch(() => {
							resonanceStore.actions.add(frame);
						});
						return;
					}

					const state = EnvelopeState.getRootAsEnvelopeState(buffer);

					storeBatch(() => {
						dispatchEnvelopeState(state);
					});
				} catch (err) {
					errorStore.setState(() => err as Event);
				}
			}
		});

		wsWorker.postMessage({ type: "CONNECT", url: wsUrl });

		const unsubscribeFocus = focusStore.subscribe((symbol: string) => {
			wsWorker.postMessage({ type: "FOCUS", symbol });
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			wsWorker.postMessage({ type: "DISCONNECT" });
			wsWorker.terminate();
			globalWsWorker = null;
		};
	}, []);

	return null;
};
