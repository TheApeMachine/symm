import { batch as storeBatch } from "@tanstack/react-store";
import * as flatbuffers from "flatbuffers";
import { useEffect } from "react";
import { addResonanceReading } from "#/collections/app";
import { topologyStore } from "#/collections/topology";
import { FluidRecordReader } from "#/components/fluid-3d/record";
import { Envelope } from "#/providers/telemetry/telemetry/envelope";
import { EnvelopeBoundaryStamp } from "#/providers/telemetry/telemetry/envelope-boundary-stamp";
import { EnvelopeStateFrame } from "#/providers/telemetry/telemetry/envelope-state-frame";

const resonanceChannel = "resonance";
const diagnosticsChannel = "diagnostics";

const signalingURL = () =>
	import.meta.env.VITE_SYMM_WEBRTC_URL?.trim() ||
	"http://127.0.0.1:8765/webrtc/manifold";

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
decodeEnvelopeState reads the SYMM-identified Envelope wrapper each WebRTC
record carries and returns the lean EnvelopeState inside its state frame. The
resonance and diagnostics publishers each build this shape with exactly one
field populated, so the accessor below is the single shared decode.
*/
const decodeState = (bytes: Uint8Array) => {
	const buffer = new flatbuffers.ByteBuffer(bytes);

	if (!Envelope.bufferHasIdentifier(buffer)) {
		throw new Error("rtc record is missing its SYMM identifier");
	}

	const envelope = Envelope.getRootAsEnvelope(buffer);
	const frame = envelope.frame(new EnvelopeStateFrame());

	if (frame === null) {
		throw new Error("rtc record is not an EnvelopeStateFrame");
	}

	const state = frame.state();

	if (state === null) {
		throw new Error("EnvelopeStateFrame is missing its state");
	}

	return state;
};

/*
RtcFeed owns the global WebRTC transport for the two payload families that left
the websocket: the predictive-coder resonance artifact and the per-stage
diagnostics boundary trace. It is a sibling of WsFeed — mounted once at the
shell — and drains them into the same stores the websocket dispatcher used to
feed, so every consumer keeps working unchanged.

It deliberately opens its own peer connection rather than sharing the fluid
route's manifold connection: the manifold scene lives and dies with its route,
while resonance and topology must keep flowing session-wide.
*/
export const RtcFeed = () => {
	useEffect(() => {
		let disposed = false;
		const connection = new RTCPeerConnection();

		const attach = (label: string, onRecord: (state: unknown) => void) => {
			const channel = connection.createDataChannel(label, {
				ordered: false,
				maxRetransmits: 0,
			});
			const reader = new FluidRecordReader();
			channel.binaryType = "arraybuffer";
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

		attach(resonanceChannel, (record) => {
			const state = decodeState(new Uint8Array(record as ArrayBuffer));
			const resonance = state.resonance();

			if (resonance) {
				storeBatch(() => {
					addResonanceReading(resonance);
				});
			}
		});

		attach(diagnosticsChannel, (record) => {
			const state = decodeState(new Uint8Array(record as ArrayBuffer));
			const count = state.boundariesLength();

			if (count === 0) {
				return;
			}

			const stamps: EnvelopeBoundaryStamp[] = [];

			for (let index = 0; index < count; index += 1) {
				const stamp = state.boundaries(index, new EnvelopeBoundaryStamp());

				if (stamp) {
					stamps.push(stamp);
				}
			}

			topologyStore.actions.ingest(stamps);
		});

		const connect = async () => {
			try {
				await connection.setLocalDescription(await connection.createOffer());
				await waitForIceGathering(connection);

				if (disposed) {
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

				if (disposed) {
					return;
				}

				await connection.setRemoteDescription(await response.json());
			} catch (error) {
				console.error("rtc:", error);
			}
		};

		void connect();

		return () => {
			disposed = true;
			connection.close();
		};
	}, []);

	return null;
};
