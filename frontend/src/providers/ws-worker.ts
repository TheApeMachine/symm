/// <reference lib="webworker" />

import { Frame } from "#/providers/telemetry/telemetry/frame";
import { Measurement } from "#/providers/telemetry/telemetry/measurement";
import { GPUStreamRenderer } from "./gpu-stream";
import type { StreamWorkerMessage } from "./stream-protocol";
import {
	type MeasurementView,
	materializeFrameView,
	materializeMeasurement,
	measurementsTable,
	openTelemetryBatch,
	type TelemetryFrameView,
} from "./ws-views";

const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 5000;

type WorkerInbound =
	| { type: "CONNECT"; url: string }
	| { type: "DISCONNECT" }
	| { type: "PAINTED"; acknowledgeBackend: boolean }
	| { type: "FOCUS"; symbol: string }
	| {
			type: "BACKTEST";
			action: "play" | "pause" | "seek" | "select" | "hindsight";
			at?: string;
			captureId?: number;
	  }
	| { type: "POSITION_EXIT"; symbol: string }
	| StreamWorkerMessage;

type WorkerOutbound =
	| { type: "READY" }
	| { type: "ONLINE"; online: boolean }
	| { type: "DRAW_BATCH"; frames: Record<string, unknown>[] }
	| { type: "ERROR"; message: string };

const eventTypes = new Set([
	Frame.StrategyFrame,
	Frame.PositionsFrame,
	Frame.ErrorFrame,
]);

let socket: WebSocket | null = null;
let socketListeners: AbortController | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let activeUrl = "";
let focusSymbol = "";
let paintInFlight = false;
let eventQueue: TelemetryFrameView[] = [];
const stateMailboxes = new Map<Frame, TelemetryFrameView>();
const measurementMailboxes = new Map<string, MeasurementView>();
const renderers = new Map<number, GPUStreamRenderer>();
const measurementRowView = new Measurement();

const report = (err: unknown) => {
	self.postMessage({
		type: "ERROR",
		message:
			err instanceof Error ? `${err.message}\n\n${err.stack}` : String(err),
	} satisfies WorkerOutbound);
};

/*
flushPresentation advances only the DOM presentation head. Event views retain
wire order; state and measurement views are mailboxes. Dense chart ingestion
has already happened directly from FlatBuffer tables and never waits here.
*/
const flushPresentation = () => {
	if (paintInFlight) {
		return;
	}

	const frames = eventQueue.map(materializeFrameView);
	eventQueue = [];

	for (const view of stateMailboxes.values()) {
		frames.push(materializeFrameView(view));
	}

	stateMailboxes.clear();

	if (measurementMailboxes.size > 0) {
		frames.push({
			measurements: Array.from(
				measurementMailboxes.values(),
				materializeMeasurement,
			),
		});
		measurementMailboxes.clear();
	}

	if (frames.length === 0) {
		return;
	}

	paintInFlight = true;
	self.postMessage({ type: "DRAW_BATCH", frames } satisfies WorkerOutbound);
};

const resetPresentation = () => {
	eventQueue = [];
	stateMailboxes.clear();
	measurementMailboxes.clear();
	paintInFlight = false;
};

const ingestMeasurements = (view: TelemetryFrameView) => {
	const table = measurementsTable(view);

	for (let index = 0; index < table.rowsLength(); index += 1) {
		const row = table.rows(index, measurementRowView);
		const source = row?.source();
		const symbol = row?.symbol();

		if (row === null || source == null || symbol == null) {
			// A single malformed row must not abort the rest of the batch (or, under
			// the old ACK-gated writer, starve the whole socket). Skip it.
			continue;
		}

		for (const renderer of renderers.values()) {
			if (renderer.matches(source, symbol)) {
				renderer.ingest(row);
			}
		}

		measurementMailboxes.set(`${source}\u0000${symbol}`, {
			frame: view,
			row: index,
		});
	}
};

const ingest = (buffer: ArrayBuffer) => {
	for (const view of openTelemetryBatch(buffer)) {
		if (view.type === Frame.MeasurementsFrame) {
			ingestMeasurements(view);
			continue;
		}

		if (eventTypes.has(view.type)) {
			eventQueue.push(view);
			continue;
		}

		stateMailboxes.set(view.type, view);
	}

	flushPresentation();
};

const sendBacktest = (
	action: "play" | "pause" | "seek" | "select" | "hindsight",
	at?: string,
	captureId?: number,
) => {
	if (socket !== null && socket.readyState === WebSocket.OPEN) {
		socket.send(JSON.stringify({ type: `backtest.${action}`, at, captureId }));
	}
};

const sendFocus = (symbol: string) => {
	focusSymbol = symbol;

	if (socket !== null && socket.readyState === WebSocket.OPEN) {
		socket.send(JSON.stringify({ type: "focus", symbol }));
	}
};

const sendPositionExit = (symbol: string) => {
	if (socket !== null && socket.readyState === WebSocket.OPEN) {
		socket.send(JSON.stringify({ type: "position.exit", symbol }));
	}
};

const teardownSocket = () => {
	socketListeners?.abort();
	socketListeners = null;

	if (
		socket !== null &&
		(socket.readyState === WebSocket.OPEN ||
			socket.readyState === WebSocket.CONNECTING)
	) {
		socket.close();
	}

	socket = null;
};

const connect = (url: string) => {
	if (reconnectTimer !== null) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	activeUrl = url;
	teardownSocket();
	socketListeners = new AbortController();
	socket = new WebSocket(url);
	socket.binaryType = "arraybuffer";
	socket.addEventListener(
		"open",
		() => {
			attempt = 0;

			if (focusSymbol !== "") {
				sendFocus(focusSymbol);
			}

			self.postMessage({
				type: "ONLINE",
				online: true,
			} satisfies WorkerOutbound);
		},
		{ signal: socketListeners.signal },
	);
	socket.addEventListener(
		"close",
		() => {
			self.postMessage({
				type: "ONLINE",
				online: false,
			} satisfies WorkerOutbound);

			if (reconnectTimer !== null || activeUrl === "") {
				return;
			}

			const delay = Math.min(
				RECONNECT_MAX_MS,
				RECONNECT_BASE_MS * 2 ** attempt,
			);
			attempt += 1;
			reconnectTimer = setTimeout(
				() => {
					reconnectTimer = null;
					connect(activeUrl);
				},
				Math.floor(Math.random() * delay),
			);
		},
		{ signal: socketListeners.signal },
	);
	socket.addEventListener(
		"error",
		(event) => {
			if (
				event.currentTarget instanceof WebSocket &&
				event.currentTarget.readyState === WebSocket.OPEN
			) {
				event.currentTarget.close();
			}
		},
		{ signal: socketListeners.signal },
	);
	socket.addEventListener(
		"message",
		(event) => {
			try {
				if (!(event.data instanceof ArrayBuffer)) {
					throw new Error("telemetry websocket requires a binary frame");
				}

				ingest(event.data);
			} catch (err) {
				report(err);
			}
		},
		{ signal: socketListeners.signal },
	);
};

self.addEventListener("message", (event: MessageEvent<WorkerInbound>) => {
	const message = event.data;

	try {
		switch (message.type) {
			case "CONNECT":
				connect(message.url);
				return;
			case "DISCONNECT":
				activeUrl = "";
				resetPresentation();
				if (reconnectTimer !== null) {
					clearTimeout(reconnectTimer);
					reconnectTimer = null;
				}
				teardownSocket();
				self.postMessage({
					type: "ONLINE",
					online: false,
				} satisfies WorkerOutbound);
				return;
			case "PAINTED":
				paintInFlight = false;
				flushPresentation();
				return;
			case "FOCUS":
				sendFocus(message.symbol);
				return;
			case "BACKTEST":
				sendBacktest(message.action, message.at, message.captureId);
				return;
			case "POSITION_EXIT":
				sendPositionExit(message.symbol);
				return;
			case "REGISTER_STREAM":
				renderers.set(
					message.registration.id,
					new GPUStreamRenderer(message.registration),
				);
				return;
			case "RESIZE_STREAM":
				renderers
					.get(message.id)
					?.resize(message.width, message.height, message.pixelRatio);
				return;
			case "UPDATE_STREAM":
				renderers.get(message.id)?.updateMetric(message.metric);
				return;
			case "UNREGISTER_STREAM":
				renderers.get(message.id)?.dispose();
				renderers.delete(message.id);
				return;
			default: {
				const exhaustive: never = message;
				return exhaustive;
			}
		}
	} catch (err) {
		report(err);
	}
});

self.postMessage({ type: "READY" } satisfies WorkerOutbound);
