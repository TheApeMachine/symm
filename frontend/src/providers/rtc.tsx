import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import {
	RingBuffer,
	signals,
	symbolsAtom,
	updateClock,
} from "#/collections/app";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import { Message } from "#/providers/telemetry/telemetry/message";
import type { ResonanceT } from "#/providers/telemetry/telemetry/resonance";
import { ResonanceFrame } from "#/providers/telemetry/telemetry/resonance-frame";

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
