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
import { Message } from "#/providers/telemetry/telemetry/message";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import { dispatchMeasurements } from "#/providers/websocket";
import { hubBaseUrl } from "#/lib/hub";

const telemetryChannel = "telemetry";

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
	return `${hubBaseUrl()}/webrtc/manifold`;
};

const setTransport = (
	status: "ONLINE" | "CONNECTING" | "OFFLINE",
) => {
	onlineAtom.set(status);
};

export const dispatchMeasurementsBuffer = (buffer: flatbuffers.ByteBuffer) => {
	if (Message.bufferHasIdentifier(buffer)) {
		const message = Message.getRootAsMessage(buffer);
		const frame = message.frame(new MeasurementsFrame());
		if (frame) {
			dispatchMeasurements(frame);
		}
	} else {
		const frame = MeasurementsFrame.getRootAsMeasurementsFrame(buffer);
		dispatchMeasurements(frame);
	}
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
				onRecord: (record: ArrayBuffer) => void,
			) => {
				const channel = connection.createDataChannel(label, {
					ordered: false,
					maxRetransmits: 0,
				});
				channel.binaryType = "arraybuffer";

				channel.addEventListener("open", onChannelOpen);

				channel.addEventListener("message", (event) => {
					try {
						if (!(event.data instanceof ArrayBuffer)) {
							throw new Error(`${label} received a non-binary message`);
						}

						onRecord(event.data);
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

			openChannel(telemetryChannel, (record) => {
				const bytes = new Uint8Array(record as ArrayBuffer);
				const buffer = new flatbuffers.ByteBuffer(bytes);
				dispatchMeasurementsBuffer(buffer);
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
