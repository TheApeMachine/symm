import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import {
	observeSymbols,
	onlineAtom,
	resonanceStore,
	RingBuffer,
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

const setTransport = (
	status: "ONLINE" | "CONNECTING" | "OFFLINE",
) => {
	onlineAtom.set(status)
};

export const dispatchResonanceRow = (row: {
	symbol: () => string | null;
	unpack: () => MeasurementT | ResonanceT;
}) => {
	const symbol = row.symbol() ?? "";

	if (symbol && !symbolsAtom.get().includes(symbol)) {
		observeSymbols([symbol]);
	}

	let ring = resonanceStore.state[symbol];

	if (!ring) {
		ring = new RingBuffer<MeasurementT | ResonanceT>(50);
		resonanceStore.state[symbol] = ring;
	}

	const unpacked = row.unpack();
	ring.add(unpacked);

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
			resonanceStore.setState((prev) => ({ ...prev }));
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
	useEffect(() => {
		let disposed = false;
		let peer: RTCPeerConnection | null = null;
		let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
		let reconnectAttempts = 0;
		const destroy = () => {
			if (peer === null) {
				return;
			}

			const closing = peer;
			// Null the handle before close() so the synchronous
			// connectionstatechange event close() dispatches sees connection !==
			// peer and never recurses into fail()/scheduleReconnect().
			peer = null;
			closing.close();
		};

		const scheduleReconnect = () => {
			if (reconnectTimer !== null) {
				return;
			}

			const delay = Math.min(
				RECONNECT_BASE_MS * 2 ** reconnectAttempts,
				RECONNECT_MAX_MS,
			);

			reconnectTimer = setTimeout(() => {
				reconnectTimer = null;
				reconnectAttempts += 1;
				void connect();
			}, delay);
		};

		const fail = () => {
			if (disposed) {
				return;
			}

			destroy();
			scheduleReconnect();
		};

		const connect = async () => {
			if (disposed) {
				return;
			}

			destroy();

			const connection = new RTCPeerConnection();
			peer = connection;

			// The transport is healthy only once both data channels reach OPEN.
			// Backoff resets at that point so a steady connection never widens into
			// an ever-growing delay; a churning one (channel opens but peer drops)
			// keeps the exponential ramp.
			let openChannels = 0;

			const onChannelOpen = () => {
				openChannels += 1;

				if (openChannels < 1) {
					return;
				}

				reconnectAttempts = 0;
				setTransport("ONLINE");
			};

			const openChannel = (
				label: string,
				onRecord: (state: unknown) => void,
			) => {
				const channel = connection.createDataChannel(label, {
					ordered: false,
					maxRetransmits: 0,
				});
				const reader = new FluidRecordReader();
				channel.binaryType = "arraybuffer";

				channel.addEventListener("open", onChannelOpen);

				channel.addEventListener("message", (event) => {
					try {
						if (!(event.data instanceof ArrayBuffer)) {
							throw new Error(`${label} received a non-binary message`);
						}

						const record = reader.push(event.data);

						if (record !== null) {
							onRecord(record);
						}
					} catch (error) {
						console.error("rtc:", error);
					}
				});
			};

			connection.addEventListener("connectionstatechange", () => {
				if (connection !== peer) {
					return;
				}

				if (connection.connectionState === "connected") {
					setTransport("ONLINE");
					reconnectAttempts = 0;
				}

				if (TERMINAL_CONNECTION_STATES.has(connection.connectionState)) {
					fail();
				}
			});

			openChannel(resonanceChannel, (record) => {
				const bytes = new Uint8Array(record as ArrayBuffer);
				const buffer = new flatbuffers.ByteBuffer(bytes);
				dispatchResonanceBuffer(buffer);
			});

			try {
				await connection.setLocalDescription(await connection.createOffer());
				await waitForIceGathering(connection);

				if (disposed || connection !== peer) {
					return;
				}

				const offer = connection.localDescription;

				if (offer === null) {
					throw new Error("rtc offer has no local description");
				}

				const response = await fetch(signalingURL(), {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ type: offer.type, sdp: offer.sdp }),
				});

				if (!response.ok) {
					throw new Error(`rtc signaling failed with ${response.status}`);
				}

				if (disposed || connection !== peer) {
					return;
				}

				await connection.setRemoteDescription(await response.json());
			} catch (error) {
				console.error("rtc:", error);

				if (connection === peer) {
					fail();
				}
			}
		};

		void connect();

		return () => {
			disposed = true;

			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
				reconnectTimer = null;
			}

			destroy();
		};
	}, []);

	return null;
};
