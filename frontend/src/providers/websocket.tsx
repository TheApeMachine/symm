import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	addMeasurement,
	appStore,
	categoryStore,
	cognitionStore,
	decisionStore,
	equityStore,
	errorStore,
	focusStore,
	onlineStore,
	opportunityStore,
	positionStore,
	strategyStore,
	tickCountStore,
	tickStore,
} from "#/collections/app";

import { Decision } from "#/providers/telemetry/telemetry/decision";
import { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";
import { EquityFrame } from "#/providers/telemetry/telemetry/equity-frame";
import { PositionsFrame } from "#/providers/telemetry/telemetry/positions-frame";

let globalWsWorker: Worker | null = null;

export const getWsWorker = () => globalWsWorker;

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

	// The engine tick is a whole-market fact, but only Ticker envelopes carry a
	// stamped tick — trade/level3/futures envelopes serialize tick=0. A zero
	// tick means "no engine tick on this envelope", so skip it instead of
	// resetting the counter back to zero on every non-ticker frame.
	const tick = state.tick();
	if (tick > 0n) {
		tickCountStore.setState(() => Number(tick));
	}

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

	// The account valuation rides ticker envelopes, so the balance recovers on
	// the next market event after a connect or refresh. A fresh view per push:
	// the ring holds the decoded rows, and a shared view object would leave
	// every stored row aliasing the last one read.
	const equity = state.equity(new EquityFrame());
	if (equity) equityStore.actions.add(equity);

	// Open positions ride the envelope exactly like equity, so the positions
	// panel recovers on the next market event.
	const positions = state.positions(new PositionsFrame());
	if (positions) positionStore.actions.add(positions);

	const cognition = state.cognition();
	const cognitionSymbol = cognition?.symbol();
	if (cognition && cognitionSymbol) {
		cognitionStore.actions.add(cognitionSymbol, cognition);
	}

	const strategy = state.strategy();
	if (strategy) {
		strategyStore.actions.add(strategy);

		// Each decision is unpacked here, at the boundary. The flatbuffer
		// Decision is a cursor into this message's buffer: retaining one would
		// leave the store aliasing whichever decision was read last, and would
		// dangle once the buffer is replaced.
		for (let index = 0; index < strategy.decisionsLength(); index++) {
			const decision = strategy.decisions(index, new Decision());

			if (decision) decisionStore.actions.add(decision.unpack());
		}
	}

	for (let i = 0; i < state.categoriesLength(); i++) {
		const category = state.categories(i);
		if (category) categoryStore.actions.add(category);
	}

	for (let i = 0; i < state.opportunitiesLength(); i++) {
		const opportunity = state.opportunities(i);
		if (opportunity) opportunityStore.actions.add(opportunity);
	}

	// The resonance artifact and the per-stage boundary trace no longer ride
	// this socket: they arrive on their own WebRTC channels (see providers/rtc),
	// where the same stores are fed. The websocket carries only the lean,
	// latency-relevant envelope state.
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
