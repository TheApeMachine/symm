import { FluidRecordReader } from "./record";
import {
	decodeManifold,
	type FluidFields,
	type FluidParticleFrame,
	type FluidPhase,
} from "./wire";

const manifoldChannel = "manifold";

export type FluidFeedHandlers = {
	onFields: (fields: FluidFields) => void;
	onParticles: (particles: FluidParticleFrame) => void;
	onPhase: (phase: FluidPhase) => void;
	onState: (state: RTCPeerConnectionState | "connecting") => void;
	onError: (error: Error) => void;
};


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

const errorValue = (value: unknown) =>
	value instanceof Error ? value : new Error(String(value));

/*
FluidWebRTCFeed owns one peer connection carrying the manifold channel and
decodes each ManifoldFrame once into the fields/particles/phase views the
viewer paints.
*/
export class FluidWebRTCFeed {
	private connection: RTCPeerConnection | null = null;

	constructor(private readonly handlers: FluidFeedHandlers) {}

	async connect() {
		this.close();
		this.handlers.onState("connecting");
		const connection = new RTCPeerConnection();
		this.connection = connection;
		connection.addEventListener("connectionstatechange", () => {
			if (this.connection === connection) {
				this.handlers.onState(connection.connectionState);
			}
		});

		const channel = connection.createDataChannel(manifoldChannel, {
			ordered: true,
		});
		const reader = new FluidRecordReader();
		channel.binaryType = "arraybuffer";
		channel.addEventListener("message", (event) => {
			try {
				if (!(event.data instanceof ArrayBuffer)) {
					throw new Error(`${channel.label} received a non-binary message`);
				}

				const record = reader.push(event.data);

				if (record !== null) {
					const { fields, particles, phase } = decodeManifold(
						new Uint8Array(record),
					);
					this.handlers.onFields(fields);
					this.handlers.onParticles(particles);
					this.handlers.onPhase(phase);
				}
			} catch (error) {
				this.handlers.onError(errorValue(error));
			}
		});

		try {
			await connection.setLocalDescription(await connection.createOffer());
			await waitForIceGathering(connection);

			if (this.connection !== connection) {
				return;
			}

			const offer = connection.localDescription;

			if (offer === null) {
				throw new Error("fluid WebRTC offer has no local description");
			}

			const response = await fetch(signalingURL(), {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ type: offer.type, sdp: offer.sdp }),
			});

			if (!response.ok) {
				throw new Error(
					`fluid WebRTC signaling failed with ${response.status}`,
				);
			}

			if (this.connection !== connection) {
				return;
			}

			await connection.setRemoteDescription(await response.json());
		} catch (error) {
			if (this.connection === connection) {
				this.handlers.onError(errorValue(error));
				this.close();
			}
		}
	}

	close() {
		const connection = this.connection;
		this.connection = null;
		connection?.close();
	}
}
