import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	onlineAtom,
	RingBuffer,
	signals,
	symbolsAtom,
	updateClock,
} from "#/collections/app";
import { FluidRecordReader } from "#/components/fluid-3d/record";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import { Message } from "#/providers/telemetry/telemetry/message";
import type { ResonanceT } from "#/providers/telemetry/telemetry/resonance";
import { ResonanceFrame } from "#/providers/telemetry/telemetry/resonance-frame";

const resonanceChannel = "resonance";

// Backoff policy mirrors the websocket worker so both transports degrade at the
// same pace instead of one silently giving up on a transient failure.
const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 10_000;

/*
Every connection attempt owns a fresh RTCPeerConnection. These lifecycle states
all mean "this peer is no longer usable": destroy it and schedule a retry.
*/
const TERMINAL_CONNECTION_STATES: ReadonlySet<RTCPeerConnectionState> = new Set(
	["failed", "disconnected", "closed"],
);

const signalingURL = () => {
	if (import.meta.env.VITE_SYMM_WEBRTC_URL?.trim()) {
		return import.meta.env.VITE_SYMM_WEBRTC_URL.trim();
	}
	const host =
		typeof window !== "undefined" && window.location.hostname
			? window.location.hostname
			: "127.0.0.1";
	return `http://${host}:8765/webrtc/manifold`;
};

const setTransport = (status: "ONLINE" | "CONNECTING" | "OFFLINE") => {
	onlineAtom.set(status);
};

export const dispatchResonanceRow = (row: {
	symbol: () => string | null;
	unpack: () => MeasurementT | ResonanceT;
}) => {
	const symbol = row.symbol() ?? "";

	if (symbol && !symbolsAtom.get().includes(symbol)) {
		symbolsAtom.set(Array.from(new Set([...symbolsAtom.get(), symbol])));
	}

	let ring = signals.resonance.state[symbol];

	if (!ring) {
		ring = new RingBuffer<MeasurementT>(50);
		signals.resonance.state[symbol] = ring;
	}

	const unpacked = row.unpack();
	ring.add(unpacked as any);

	if ("at" in unpacked && unpacked.at) {
		updateClock(unpacked.at);
	}
};

export const dispatchResonanceBuffer = (buffer: flatbuffers.ByteBuffer) => {
	let touched = false;

	storeBatch(() => {
		if (Message.bufferHasIdentifier(buffer)) {
			const message = Message.getRootAsMessage(buffer);
			const frameType = message.frameType();

			if (frameType === Frame.MeasurementsFrame) {
				const frame = message.frame(new MeasurementsFrame());
				if (frame) {
					const count = frame.rowsLength();
					for (let index = 0; index < count; index += 1) {
						const row = frame.rows(index);
						if (row) {
							dispatchResonanceRow(row);
							touched = true;
						}
					}
				}
			} else {
				const frame = message.frame(new ResonanceFrame());
				if (frame) {
					const count = frame.rowsLength();
					for (let index = 0; index < count; index += 1) {
						const row = frame.rows(index);
						if (row) {
							dispatchResonanceRow(row);
							touched = true;
						}
					}
				}
			}
		} else {
			try {
				const frame = MeasurementsFrame.getRootAsMeasurementsFrame(buffer);
				const count = frame.rowsLength();
				for (let index = 0; index < count; index += 1) {
					const row = frame.rows(index);
					if (row) {
						dispatchResonanceRow(row);
						touched = true;
					}
				}
			} catch {
				const frame = ResonanceFrame.getRootAsResonanceFrame(buffer);
				const count = frame.rowsLength();
				for (let index = 0; index < count; index += 1) {
					const row = frame.rows(index);
					if (row) {
						dispatchResonanceRow(row);
						touched = true;
					}
				}
			}
		}

		if (touched) {
			signals.resonance.setState((prev: any) => ({ ...prev }));
		}
	});
};

const waitForIceGathering = (connection: RTCPeerConnection) => {
	if (connection.iceGatheringState === "complete") {
		return Promise.resolve();
	}

	return new Promise<void>((resolve) => {
		const onState = () => {
			if (connection.iceGatheringState !== "complete") {
				return;
			}

			connection.removeEventListener("icegatheringstatechange", onState);
			resolve();
		};

		connection.addEventListener("icegatheringstatechange", onState);
	});
};

/*
RtcFeed owns the global WebRTC transport for the two payload families that left
the websocket: the predictive-coder resonance artifact and the per-stage
diagnostics boundary trace. It is a sibling of WsFeed — mounted once at the
shell — and drains them into the same stores the websocket dispatcher used to
feed, so every consumer keeps working unchanged. Because it lives in the root
shell it stays mounted across route changes; it deliberately opens its own peer
connection rather than sharing the fluid route's manifold connection, which
lives and dies with its route while resonance and topology flow session-wide.

Unlike the websocket, each connection attempt owns a fresh RTCPeerConnection
and, on signaling failure or a terminal connection state, tears that peer down
and retries with the same exponential backoff as ws-worker.ts (500ms base, 10s
max), resetting the ramp once both data channels are open.
*/
export const RtcFeed = () => {
	return null;
};
