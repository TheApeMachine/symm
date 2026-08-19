import { FluidRecordReader } from "#/components/fluid-3d/record";
import type { DiagnosticsFrame } from "#/collections/types";

const diagnosticsChannel = "diagnostics";
const textDecoder = new TextDecoder();

export type DiagnosticsFeedHandlers = {
	onFrame: (frame: DiagnosticsFrame) => void;
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
DiagnosticsWebRTCFeed owns one WebRTC peer that carries the replaceable
diagnostics snapshot. It avoids routing diagnostics through the orchestrating
dashboard WebSocket store, keeping the pipeline's high-frequency signal frames
off the WS bus that was being saturated.
*/
export class DiagnosticsWebRTCFeed {
	private connection: RTCPeerConnection | null = null;

	constructor(private readonly handlers: DiagnosticsFeedHandlers) {}

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

		const channel = connection.createDataChannel(diagnosticsChannel, {
			ordered: true,
		});
		const reader = new FluidRecordReader();
		channel.binaryType = "arraybuffer";
		channel.addEventListener("message", (event) => {
			try {
				if (!(event.data instanceof ArrayBuffer)) {
					throw new Error("diagnostics received a non-binary message");
				}

				const record = reader.push(event.data);

				if (record !== null) {
					const envelope = JSON.parse(textDecoder.decode(record)) as unknown;
					const frame =
						envelope !== null &&
						typeof envelope === "object" &&
						"diagnostics" in envelope &&
						typeof (envelope as Record<string, unknown>).diagnostics ===
							"object"
							? ((envelope as Record<string, unknown>).diagnostics as DiagnosticsFrame)
							: (envelope as DiagnosticsFrame);

					this.handlers.onFrame(frame);
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
				throw new Error("diagnostics WebRTC offer has no local description");
			}

			const response = await fetch(signalingURL(), {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ type: offer.type, sdp: offer.sdp }),
			});

			if (!response.ok) {
				throw new Error(
					`diagnostics WebRTC signaling failed with ${response.status}`,
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
