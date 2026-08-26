import * as flatbuffers from "flatbuffers";
import {
	diagnosticStore,
	errorStore,
	onlineStore,
	RingBuffer,
} from "#/collections/app";
import { FluidRecordReader } from "#/components/fluid-3d/record";
import { DiagnosticsFrame } from "#/providers/telemetry/telemetry/diagnostics-frame";
import { Envelope } from "#/providers/telemetry/telemetry/envelope";

const signalingURL = () =>
	import.meta.env.VITE_SYMM_WEBRTC_URL?.trim() ||
	"http://127.0.0.1:8765/webrtc/manifold";

export const connectWebRTC = async (): Promise<void> => {
	const connection = new RTCPeerConnection();

	connection.addEventListener("connectionstatechange", () => {
		onlineStore.setState(() =>
			connection.connectionState === "connected" ? "ONLINE" : "OFFLINE",
		);
	});

	const channel = connection.createDataChannel("diagnostics", {
		ordered: true,
	});
	channel.binaryType = "arraybuffer";
	const reader = new FluidRecordReader();

	channel.addEventListener("message", (event) => {
		try {
			if (!(event.data instanceof ArrayBuffer)) {
				throw new Error("diagnostics received a non-binary message");
			}

			const record = reader.push(event.data);
			if (record === null) return;

			const buffer = new flatbuffers.ByteBuffer(new Uint8Array(record));
			if (!Envelope.bufferHasIdentifier(buffer)) return;

			const envelope = Envelope.getRootAsEnvelope(buffer);
			const frame = envelope.frame(new DiagnosticsFrame());

			if (frame === null) return;

			diagnosticStore.setState((prev) => {
				const next = { ...prev };

				for (let i = 0; i < frame.queuesLength(); i++) {
					const row = frame.queues(i);
					if (row === null) continue;

					const name = row.name();
					if (name === null) continue;

					if (next[name] === undefined) {
						next[name] = new RingBuffer(1);
					}

					next[name].add(row);
				}

				return next;
			});
		} catch (err) {
			errorStore.setState(() => err as Event);
		}
	});

	const offer = await connection.createOffer();
	await connection.setLocalDescription(offer);

	if (connection.iceGatheringState !== "complete") {
		await new Promise<void>((resolve) => {
			const check = () => {
				if (connection.iceGatheringState === "complete") {
					connection.removeEventListener("icegatheringstatechange", check);
					resolve();
				}
			};

			connection.addEventListener("icegatheringstatechange", check);
		});
	}

	const response = await fetch(signalingURL(), {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(connection.localDescription),
	});

	await connection.setRemoteDescription(await response.json());
};
