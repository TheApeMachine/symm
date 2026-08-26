import * as flatbuffers from "flatbuffers";
import type {
	ClockSnapshot,
	DiagnosticsFrame as DiagnosticsFrameType,
	ErrorSnapshot,
	GoroutineOwner,
	HopSnapshot,
	QueueSnapshot,
} from "#/collections/types";
import { FluidRecordReader } from "#/components/fluid-3d/record";
import { DiagnosticClock } from "#/providers/telemetry/telemetry/diagnostic-clock";
import { DiagnosticError } from "#/providers/telemetry/telemetry/diagnostic-error";
import { DiagnosticGoroutine } from "#/providers/telemetry/telemetry/diagnostic-goroutine";
import { DiagnosticHop } from "#/providers/telemetry/telemetry/diagnostic-hop";
import { DiagnosticPass } from "#/providers/telemetry/telemetry/diagnostic-pass";
import { DiagnosticQueue } from "#/providers/telemetry/telemetry/diagnostic-queue";
import { DiagnosticsFrame } from "#/providers/telemetry/telemetry/diagnostics-frame";
import { Envelope } from "#/providers/telemetry/telemetry/envelope";

const diagnosticsChannel = "diagnostics";

const clockObj = new DiagnosticClock();
const hopObj = new DiagnosticHop();
const queueObj = new DiagnosticQueue();
const errorObj = new DiagnosticError();
const passObj = new DiagnosticPass();
const goroutineObj = new DiagnosticGoroutine();

export const diagnosticsFrameFromFlatBuffer = (
	frame: DiagnosticsFrame,
): DiagnosticsFrameType => {
	const stages: ClockSnapshot[] = [];
	for (let i = 0; i < frame.stagesLength(); i++) {
		const s = frame.stages(i, clockObj);
		if (!s) continue;
		stages.push({
			name: s.name() ?? "",
			count: Number(s.count()),
			total_ns: Number(s.totalNs()),
			last_ns: Number(s.lastNs()),
			max_ns: Number(s.maxNs()),
			last_at_ns: Number(s.lastAtNs()),
			active: Number(s.active()),
		});
	}

	const hops: HopSnapshot[] = [];
	for (let i = 0; i < frame.hopsLength(); i++) {
		const h = frame.hops(i, hopObj);
		if (!h) continue;
		hops.push({
			from: h.from() ?? "",
			to: h.to() ?? "",
			count: Number(h.count()),
			total_ns: Number(h.totalNs()),
			last_ns: Number(h.lastNs()),
			max_ns: Number(h.maxNs()),
		});
	}

	const queues: QueueSnapshot[] = [];
	for (let i = 0; i < frame.queuesLength(); i++) {
		const q = frame.queues(i, queueObj);
		if (!q) continue;
		const writers: string[] = [];
		for (let w = 0; w < q.writersLength(); w++) {
			const writer = q.writers(w);
			if (writer) writers.push(writer);
		}
		const readers: string[] = [];
		for (let r = 0; r < q.readersLength(); r++) {
			const reader = q.readers(r);
			if (reader) readers.push(reader);
		}
		queues.push({
			name: q.name() ?? "",
			kind: (q.kind() as QueueSnapshot["kind"]) ?? "rail",
			writers,
			readers,
			depth: Number(q.depth()),
			cap: Number(q.capacity()),
			high_water: Number(q.highWater()),
			symbols: Number(q.symbols()),
		});
	}

	const errors: ErrorSnapshot[] = [];
	for (let i = 0; i < frame.errorsLength(); i++) {
		const e = frame.errors(i, errorObj);
		if (!e) continue;
		errors.push({
			source: e.source() ?? "",
			message: e.message() ?? "",
			caller: e.caller() ?? undefined,
			at_ns: Number(e.atNs()),
		});
	}

	const goroutines: GoroutineOwner[] = [];
	for (let i = 0; i < frame.goroutinesLength(); i++) {
		const g = frame.goroutines(i, goroutineObj);
		if (!g) continue;
		goroutines.push({
			owner: g.owner() ?? "",
			count: Number(g.count()),
			state: g.state() ?? undefined,
		});
	}

	const pass = frame.pass(passObj);

	return {
		status: (frame.status() as DiagnosticsFrameType["status"]) ?? "flowing",
		enabled: frame.enabled(),
		at_ns: Number(frame.atNs()),
		started_ns: Number(frame.startedNs()),
		stages,
		hops,
		queues,
		errors,
		goroutines,
		pass: pass
			? {
					state: (pass.state() as "idle" | "running" | "blocked") ?? "idle",
					in_flight_ns: Number(pass.inFlightNs()),
					last_pass_ns: Number(pass.lastPassNs()),
					since_last_ns: Number(pass.sinceLastNs()),
				}
			: { state: "idle" },
	};
};


export type DiagnosticsFeedHandlers = {
	onFrame: (frame: DiagnosticsFrameType) => void;
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
					const buffer = new flatbuffers.ByteBuffer(new Uint8Array(record));
					if (Envelope.bufferHasIdentifier(buffer)) {
						const envelope = Envelope.getRootAsEnvelope(buffer);
						const frame = envelope.frame(new DiagnosticsFrame());
						if (frame) {
							this.handlers.onFrame(diagnosticsFrameFromFlatBuffer(frame));
						}

					}
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
