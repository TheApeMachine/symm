import * as flatbuffers from "flatbuffers";
import { Envelope } from "#/providers/telemetry/telemetry/envelope";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import type { JSONSerializable } from "#/components/ui/paint";

const BATCH_HEADER_BYTES = 4;
const timestampFields = new Set([
	"at",
	"observedFrom",
	"peerAt",
	"peerObservedFrom",
	"entryAt",
	"exitAt",
	"position",
	"startedAt",
	"endedAt",
	"buyAt",
	"sellAt",
	"signalAt",
	"remFrom",
	"remThrough",
]);
const namedNumberFields = new Set([
	"alternatives",
	"currentState",
	"distribution",
	"metadata",
	"predictions",
	"state",
]);
const fieldNames: Record<string, string> = {
	allocationHaircut: "allocation_haircut",
	allocationHaircutReason: "allocation_haircut_reason",
	confidenceBaseline: "confidence_baseline",
	entryBaseline: "entry_baseline",
	exitBaseline: "exit_baseline",
	noiseScore: "noiseScore",
	profitLine: "profit_line",
	armAt: "arm_at",
	lockFloor: "lock_floor",
	triggerReason: "trigger_reason",
	triggerMark: "trigger_mark",
	surgeArmed: "surge_armed",
	lastMove: "last_move",
	surgeMove: "surge_move",
	momentumFloor: "momentum_floor",
	sellableQty: "sellable_qty",
	entryAt: "entry_at",
	exitAt: "exit_at",
	entryPrice: "entry_price",
	entryFee: "entry_fee",
	exitPrice: "exit_price",
	exitFee: "exit_fee",
	profitThreshold: "profit_threshold",
	returnPct: "return_pct",
	isOpportunity: "is_opportunity",
	reservationId: "reservation_id",
};

const timestamp = (nanoseconds: bigint): string | undefined => {
	if (nanoseconds === 0n) {
		return undefined;
	}

	return new Date(Number(nanoseconds / 1_000_000n)).toISOString();
};

const plain = (value: unknown, field = ""): JSONSerializable | undefined => {
	if (value === null || value === undefined) {
		return undefined;
	}

	if (typeof value === "bigint") {
		return timestampFields.has(field) ? timestamp(value) : Number(value);
	}

	if (
		typeof value === "string" ||
		typeof value === "number" ||
		typeof value === "boolean"
	) {
		return value;
	}

	if (Array.isArray(value)) {
		if (namedNumberFields.has(field)) {
			return Object.fromEntries(
				value.flatMap((entry) => {
					if (entry === null || typeof entry !== "object") {
						return [];
					}

					const named = entry as { name?: string; value?: unknown };
					return named.name === undefined
						? []
						: [[named.name, plain(named.value)]];
				}),
			) as JSONSerializable;
		}

		if (field === "metrics") {
			return Object.fromEntries(
				value.flatMap((entry) => {
					if (entry === null || typeof entry !== "object") {
						return [];
					}

					const metric = entry as Record<string, unknown>;
					const name = metric.name;

					if (typeof name !== "string") {
						return [];
					}

					return [[name, {
						raw: plain(metric.raw),
						...(metric.hasNormalized === true
							? { normalized: plain(metric.normalized) }
							: {}),
						...(metric.unit ? { unit: plain(metric.unit) } : {}),
					}]];
				}),
			) as JSONSerializable;
		}

		return value.flatMap((entry) => {
			const converted = plain(entry, field);
			return converted === undefined ? [] : [converted];
		});
	}

	if (typeof value !== "object") {
		return undefined;
	}

	if (field === "metadata") {
		const metadata = value as Record<string, unknown>;
		const entries: Array<[string, JSONSerializable | undefined]> = [];

		for (const groupName of ["numbers", "strings", "bools"]) {
			const group = metadata[groupName];

			if (!Array.isArray(group)) {
				continue;
			}

			for (const entry of group) {
				if (entry !== null && typeof entry === "object") {
					const named = entry as { name?: string; value?: unknown };

					if (named.name !== undefined) {
						entries.push([named.name, plain(named.value)]);
					}
				}
			}
		}

		if (Array.isArray(metadata.stringLists)) {
			for (const entry of metadata.stringLists) {
				if (entry !== null && typeof entry === "object") {
					const named = entry as { name?: string; values?: unknown };

					if (named.name !== undefined) {
						entries.push([named.name, plain(named.values)]);
					}
				}
			}
		}

		return Object.fromEntries(entries) as JSONSerializable;
	}

	const converted: Record<string, JSONSerializable | undefined> = {};

	for (const [name, entry] of Object.entries(value)) {
		if (name.startsWith("has") && typeof entry === "boolean") {
			continue;
		}

		const outputName = fieldNames[name] ?? name;
		const output = plain(entry, name);

		if (output !== undefined) {
			converted[outputName] = output;
		}
	}

	return converted;
};

const keyedRows = (
	rows: JSONSerializable | undefined,
	identity: string,
): JSONSerializable => {
	if (!Array.isArray(rows)) {
		return {};
	}

	return Object.fromEntries(
		rows.flatMap((row) => {
			if (row === null || typeof row !== "object" || Array.isArray(row)) {
				return [];
			}

			const key = row[identity];
			return typeof key === "string" ? [[key, row]] : [];
		}),
	) as JSONSerializable;
};

const decodeEnvelope = (bytes: Uint8Array): Record<string, JSONSerializable> => {
	const byteBuffer = new flatbuffers.ByteBuffer(bytes);

	if (!Envelope.bufferHasIdentifier(byteBuffer)) {
		throw new Error("telemetry frame has no SYMM FlatBuffers identifier");
	}

	const envelope = Envelope.getRootAsEnvelope(byteBuffer).unpack();
	const payload = plain(envelope.frame);

	if (payload === undefined || payload === null || typeof payload !== "object" || Array.isArray(payload)) {
		throw new Error("telemetry envelope has no frame payload");
	}

	switch (envelope.frameType) {
		case Frame.MeasurementsFrame:
			return { measurements: payload.rows ?? [] };
		case Frame.TickFrame:
			return { tick: payload };
		case Frame.EquityFrame:
			return { equity: payload };
		case Frame.BalancesFrame:
			return { balances: keyedRows(payload.balances, "asset") };
		case Frame.ResonanceFrame:
			return { resonance: payload.rows ?? [] };
		case Frame.CognitionFrame:
			return { cognition: keyedRows(payload.rows, "symbol") };
		case Frame.CausalFrame:
			return { causal: payload.rows ?? [] };
		case Frame.GraphFrame:
			return {
				graph: {
					...payload,
					nodes: keyedRows(payload.nodes, "id"),
				},
			};
		case Frame.StrategyFrame:
			return { strategy: payload };
		case Frame.PositionsFrame:
			return { positions: payload.rows ?? [] };
		case Frame.RegulatorFrame:
			return { regulator: payload };
		case Frame.BacktestFrame:
			return { backtest: payload };
		case Frame.HindsightFrame:
			return { hindsight: payload };
		case Frame.ErrorFrame:
			return { error: payload };
		default:
			throw new Error(`unsupported telemetry schema tag ${envelope.frameType}`);
	}
};

export const decodeTelemetryBatch = (
	batch: ArrayBuffer,
): Record<string, JSONSerializable>[] => {
	const view = new DataView(batch);

	if (view.byteLength < BATCH_HEADER_BYTES) {
		throw new Error("telemetry batch is truncated");
	}

	const count = view.getUint32(0, true);
	const frames: Record<string, JSONSerializable>[] = [];
	let offset = BATCH_HEADER_BYTES;

	for (let index = 0; index < count; index += 1) {
		if (offset + BATCH_HEADER_BYTES > view.byteLength) {
			throw new Error("telemetry frame header is truncated");
		}

		const length = view.getUint32(offset, true);
		offset += BATCH_HEADER_BYTES;

		if (offset + length > view.byteLength) {
			throw new Error("telemetry frame body is truncated");
		}

		frames.push(decodeEnvelope(new Uint8Array(batch, offset, length)));
		offset += length;
	}

	if (offset !== view.byteLength) {
		throw new Error("telemetry batch has trailing bytes");
	}

	return frames;
};
