import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	appStore,
	backtestStore,
	balanceStore,
	causalStore,
	cognitionStore,
	diagnosticStore,
	diagnosticsFrameStore,
	equityStore,
	errorFrameStore,
	errorStore,
	fluidFrameStore,
	focusStore,
	graphStore,
	hindsightStore,
	measurementStore,
	onlineStore,
	positionStore,
	regulatorStore,
	resonanceStore,
	strategyStore,
	tickStore,
} from "#/collections/app";

import { BacktestFrame } from "#/providers/telemetry/telemetry/backtest-frame";
import { BalancesFrame } from "#/providers/telemetry/telemetry/balances-frame";
import { Batch } from "#/providers/telemetry/telemetry/batch";
import { CausalFrame } from "#/providers/telemetry/telemetry/causal-frame";
import { CognitionFrame } from "#/providers/telemetry/telemetry/cognition-frame";
import { DiagnosticsFrame } from "#/providers/telemetry/telemetry/diagnostics-frame";
import { Envelope } from "#/providers/telemetry/telemetry/envelope";
import { EquityFrame } from "#/providers/telemetry/telemetry/equity-frame";
import { ErrorFrame } from "#/providers/telemetry/telemetry/error-frame";
import { FluidPhaseFrame } from "#/providers/telemetry/telemetry/fluid-phase-frame";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { FrameEntry } from "#/providers/telemetry/telemetry/frame-entry";
import { GraphFrame } from "#/providers/telemetry/telemetry/graph-frame";
import { HindsightFrame } from "#/providers/telemetry/telemetry/hindsight-frame";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import { PositionsFrame } from "#/providers/telemetry/telemetry/positions-frame";
import { RegulatorFrame } from "#/providers/telemetry/telemetry/regulator-frame";
import { ResonanceFrame } from "#/providers/telemetry/telemetry/resonance-frame";
import { StrategyFrame } from "#/providers/telemetry/telemetry/strategy-frame";
import { TickFrame } from "#/providers/telemetry/telemetry/tick-frame";

let globalWsWorker: Worker | null = null;
let globalWebrtcWorker: Worker | null = null;

export const getWsWorker = () => globalWsWorker;
export const getWebrtcWorker = () => globalWebrtcWorker;

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

export const setDiagnosticsEnabled = (enabled: boolean) => {
	const baseUrl =
		import.meta.env.VITE_SYMM_WS_URL?.replace(/\/ws$/, "") ||
		"http://127.0.0.1:8765";
	globalWebrtcWorker?.postMessage({
		type: "SET_DIAGNOSTICS",
		url: baseUrl,
		enabled,
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

const defaultWebrtcUrl = () => {
	if (typeof window === "undefined")
		return "http://127.0.0.1:8765/webrtc/manifold";
	const protocol = window.location.protocol === "https:" ? "https:" : "http:";
	const host =
		!window.location.hostname || window.location.hostname === "localhost"
			? "127.0.0.1"
			: window.location.hostname;
	return `${protocol}//${host}:8765/webrtc/manifold`;
};

type FrameHandler<T> = {
	table: new () => T;
	update: (table: T) => void;
};

function frameBuilder<T>(
	table: new () => T,
	store: { actions: { add: (t: T) => void } },
): FrameHandler<T> {
	return {
		table,
		update: (t: T) => {
			store.actions.add(t);
		},
	};
}

/*
Measurement rows are flatbuffer view objects. `table.rows(r)` must mint a
fresh view per row: reusing one shared view and storing it by reference makes
every ring slot alias the same mutable object, so the "history" a kernel
sparkline reads is really just the latest row repeated (and one sparse row
blanks the whole trace).
*/
const builders: Partial<Record<Frame, FrameHandler<any>>> = {
	[Frame.MeasurementsFrame]: {
		table: MeasurementsFrame,
		update: (table: MeasurementsFrame) => {
			for (let r = 0; r < table.rowsLength(); r++) {
				const row = table.rows(r);
				if (!row) continue;
				const source = row.source() ?? "";
				const symbol = row.symbol() ?? "";
				if (!source || !symbol) continue;
				measurementStore.actions.addMeasurement(source, symbol, row);
			}
		},
	},
	[Frame.TickFrame]: frameBuilder(TickFrame, tickStore),
	[Frame.RegulatorFrame]: frameBuilder(RegulatorFrame, regulatorStore),
	[Frame.ResonanceFrame]: frameBuilder(ResonanceFrame, resonanceStore),
	[Frame.CognitionFrame]: frameBuilder(CognitionFrame, cognitionStore),
	[Frame.CausalFrame]: frameBuilder(CausalFrame, causalStore),
	[Frame.GraphFrame]: frameBuilder(GraphFrame, graphStore),
	[Frame.StrategyFrame]: frameBuilder(StrategyFrame, strategyStore),
	[Frame.PositionsFrame]: frameBuilder(PositionsFrame, positionStore),
	[Frame.BalancesFrame]: frameBuilder(BalancesFrame, balanceStore),
	[Frame.EquityFrame]: frameBuilder(EquityFrame, equityStore),
	[Frame.FluidPhaseFrame]: frameBuilder(FluidPhaseFrame, fluidFrameStore),
	[Frame.ErrorFrame]: frameBuilder(ErrorFrame, errorFrameStore),
	[Frame.BacktestFrame]: frameBuilder(BacktestFrame, backtestStore),
	[Frame.HindsightFrame]: frameBuilder(HindsightFrame, hindsightStore),
	[Frame.DiagnosticsFrame]: {
		table: DiagnosticsFrame,
		update: (table: DiagnosticsFrame) => {
			diagnosticsFrameStore.actions.add(table);
			for (let q = 0; q < table.queuesLength(); q++) {
				const row = table.queues(q);
				if (!row) continue;
				const name = row.name() ?? "";
				if (!name) continue;
				diagnosticStore.actions.updateQueue(name, row);
			}
		},
	},
};

export const WsFeed = () => {
	useEffect(() => {
		const wsWorker = new Worker(new URL("./ws-worker.ts", import.meta.url), {
			type: "module",
		});
		globalWsWorker = wsWorker;

		const webrtcWorker = new Worker(
			new URL("./webrtc-worker.ts", import.meta.url),
			{ type: "module" },
		);
		globalWebrtcWorker = webrtcWorker;

		const wsUrl = import.meta.env.VITE_SYMM_WS_URL || defaultWsUrl();
		const webrtcUrl =
			import.meta.env.VITE_SYMM_WEBRTC_URL || defaultWebrtcUrl();

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
					const batch = Batch.getRootAsBatch(buffer);
					const entry = new FrameEntry();

					storeBatch(() => {
						for (let i = 0; i < batch.framesLength(); i++) {
							const frameEntry = batch.frames(i, entry);
							if (frameEntry === null) continue;

							const handler = builders[frameEntry.frameType()];
							if (handler === undefined) continue;

							const table = frameEntry.frame(new handler.table());
							if (table === null) continue;

							handler.update(table);
						}
					});
				} catch (err) {
					errorStore.setState(() => err as Event);
				}
			}
		});

		webrtcWorker.addEventListener("message", (event: MessageEvent) => {
			const data = event.data;
			if (!data) return;

			if (data.type === "FRAME" && data.buffer instanceof ArrayBuffer) {
				try {
					const buffer = new flatbuffers.ByteBuffer(
						new Uint8Array(data.buffer),
					);
					if (Envelope.bufferHasIdentifier(buffer)) {
						const envelope = Envelope.getRootAsEnvelope(buffer);
						const frame = envelope.frame(new DiagnosticsFrame());
						if (frame) {
							storeBatch(() => {
								diagnosticsFrameStore.actions.add(frame);
								for (let q = 0; q < frame.queuesLength(); q++) {
									const row = frame.queues(q);
									if (!row) continue;
									const name = row.name() ?? "";
									if (!name) continue;
									diagnosticStore.actions.updateQueue(name, row);
								}
							});
						}
					}
				} catch (err) {
					errorStore.setState(() => err as Event);
				}
			}
		});

		wsWorker.postMessage({ type: "CONNECT", url: wsUrl });
		webrtcWorker.postMessage({ type: "CONNECT", url: webrtcUrl });

		const unsubscribeFocus = focusStore.subscribe((symbol: string) => {
			wsWorker.postMessage({ type: "FOCUS", symbol });
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			wsWorker.postMessage({ type: "DISCONNECT" });
			webrtcWorker.postMessage({ type: "DISCONNECT" });
			wsWorker.terminate();
			webrtcWorker.terminate();
			globalWsWorker = null;
			globalWebrtcWorker = null;
		};
	}, []);

	return null;
};
