import { FluidRecordReader } from "./record";
import {
	decodeFields,
	decodeParticles,
	decodePhase,
	type FluidFields,
	type FluidParticleFrame,
} from "./wire";

const fieldsChannel = "fluid-fields";
const particlesChannel = "fluid-particles";
const phaseChannel = "fluid-phase";

export type FluidFeedHandlers = {
	onFields: (fields: FluidFields) => void;
	onParticles: (particles: FluidParticleFrame) => void;
	onPhase: (frame: Record<string, unknown>) => void;
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
FluidWebRTCFeed owns one peer connection and delivers the three direct domain
publications without routing them through the dashboard WebSocket store.
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
		this.attach(
			connection.createDataChannel(fieldsChannel, { ordered: true }),
			decodeFields,
			this.handlers.onFields,
		);
		this.attach(
			connection.createDataChannel(particlesChannel, { ordered: true }),
			decodeParticles,
			this.handlers.onParticles,
		);
		this.attach(
			connection.createDataChannel(phaseChannel, { ordered: true }),
			decodePhase,
			this.handlers.onPhase,
		);

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

	private attach<T>(
		channel: RTCDataChannel,
		decode: (value: Uint8Array) => T,
		publish: (value: T) => void,
	) {
		const reader = new FluidRecordReader();
		channel.binaryType = "arraybuffer";
		channel.addEventListener("message", (event) => {
			try {
				if (!(event.data instanceof ArrayBuffer)) {
					throw new Error(`${channel.label} received a non-binary message`);
				}

				const record = reader.push(event.data);

				if (record !== null) {
					publish(decode(new Uint8Array(record)));
				}
			} catch (error) {
				this.handlers.onError(errorValue(error));
			}
		});
	}
}
