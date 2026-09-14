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
const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5_000;
const TERMINAL_CONNECTION_STATES: ReadonlySet<RTCPeerConnectionState> = new Set(
	["failed", "disconnected", "closed"],
);

export class FluidWebRTCFeed {
	private connection: RTCPeerConnection | null = null;
	private disposed = false;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private reconnectAttempts = 0;

	constructor(private readonly handlers: FluidFeedHandlers) {}

	private scheduleReconnect() {
		if (this.disposed || this.reconnectTimer !== null) {
			return;
		}

		const delay = Math.min(
			RECONNECT_BASE_MS * 2 ** this.reconnectAttempts,
			RECONNECT_MAX_MS,
		);
		this.reconnectAttempts += 1;
		this.handlers.onState("connecting");

		this.reconnectTimer = setTimeout(() => {
			this.reconnectTimer = null;
			void this.connect();
		}, delay);
	}

	async connect() {
		if (this.disposed) {
			return;
		}

		this.destroyConnection();
		this.handlers.onState("connecting");
		const connection = new RTCPeerConnection();
		this.connection = connection;

		connection.addEventListener("connectionstatechange", () => {
			if (this.connection !== connection) {
				return;
			}

			this.handlers.onState(connection.connectionState);

			if (connection.connectionState === "connected") {
				this.reconnectAttempts = 0;
			}

			if (TERMINAL_CONNECTION_STATES.has(connection.connectionState)) {
				this.scheduleReconnect();
			}
		});

		const channel = connection.createDataChannel(manifoldChannel, {
			ordered: false,
			maxRetransmits: 0,
		});
		const reader = new FluidRecordReader();
		channel.binaryType = "arraybuffer";
		channel.addEventListener("open", () => {
			console.log("[FluidRTC] data channel opened:", channel.label);
			this.reconnectAttempts = 0;
		});
		channel.addEventListener("close", () => {
			console.log("[FluidRTC] data channel closed:", channel.label);
			this.scheduleReconnect();
		});
		channel.addEventListener("error", (event) => {
			console.error("[FluidRTC] data channel error:", event);
		});

		let chunksReceived = 0;
		channel.addEventListener("message", (event) => {
			try {
				console.log(event);
				if (!(event.data instanceof ArrayBuffer)) {
					throw new Error(`${channel.label} received a non-binary message`);
				}

				chunksReceived += 1;
				if (chunksReceived === 1 || chunksReceived % 50 === 0) {
					console.log(
						`[FluidRTC] chunk ${chunksReceived} received (${event.data.byteLength} bytes)`,
					);
				}

				const record = reader.push(event.data);

				if (record !== null) {
					console.log(
						`[FluidRTC] full frame reassembled (${record.byteLength} bytes)`,
					);
					const { fields, particles, phase } = decodeManifold(
						new Uint8Array(record),
					);
					this.handlers.onFields(fields);
					this.handlers.onParticles(particles);
					this.handlers.onPhase(phase);
				}
			} catch (error) {
				console.error("[FluidRTC] message error:", error);
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
				this.destroyConnection();
				this.scheduleReconnect();
			}
		}
	}

	private destroyConnection() {
		const connection = this.connection;
		this.connection = null;
		connection?.close();
	}

	close() {
		this.disposed = true;

		if (this.reconnectTimer !== null) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}

		this.destroyConnection();
	}
}
