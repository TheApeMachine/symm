import * as flatbuffers from "flatbuffers";
import { Batch } from "#/providers/telemetry/telemetry/batch";
import { Envelope } from "#/providers/telemetry/telemetry/envelope";
import { Frame, unionToFrame } from "#/providers/telemetry/telemetry/frame";
import type { JSONSerializable } from "#/components/ui/paint";

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
};

const positionFieldNames: Record<string, string> = {
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

const plain = (
	value: unknown,
	field = "",
	frameType?: Frame,
): JSONSerializable | undefined => {
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
		if (
			namedNumberFields.has(field) &&
			!(frameType === Frame.ResonanceFrame && field === "state")
		) {
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

					return [
						[
							name,
							{
								raw: plain(metric.raw),
								...(metric.hasNormalized === true
									? { normalized: plain(metric.normalized) }
									: {}),
								...(metric.unit ? { unit: plain(metric.unit) } : {}),
							},
						],
					];
				}),
			) as JSONSerializable;
		}

		return value.flatMap((entry) => {
			const converted = plain(entry, field, frameType);
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

	const converted: Record<string, JSONSerializable> = {};
	const record = value as Record<string, unknown>;
	const names =
		field === "holding" || field === "stoploss"
			? { ...fieldNames, ...positionFieldNames }
			: fieldNames;

	for (const [name, entry] of Object.entries(value)) {
		if (name.startsWith("has") && typeof entry === "boolean") {
			continue;
		}

		if (name === "normalized" && record.hasNormalized === false) {
			continue;
		}

		if (name === "quality" && record.hasQuality === false) {
			continue;
		}

		if (name === "entropyBits" && record.hasEntropyBits === false) {
			continue;
		}

		if (name === "entropyThreshold" && record.hasEntropyThreshold === false) {
			continue;
		}

		const outputName = names[name] ?? name;
		const output = plain(entry, name, frameType);

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

const diagnosticNames: Record<string, string> = {
	atNs: "at_ns",
	startedNs: "started_ns",
	totalNs: "total_ns",
	lastNs: "last_ns",
	maxNs: "max_ns",
	lastAtNs: "last_at_ns",
	capacity: "cap",
	highWater: "high_water",
	inFlightNs: "in_flight_ns",
	lastPassNs: "last_pass_ns",
	sinceLastNs: "since_last_ns",
};

const diagnosticsPayload = (payload: Record<string, JSONSerializable>) => {
	const convert = (value: JSONSerializable): JSONSerializable => {
		if (Array.isArray(value)) {
			return value.map(convert);
		}

		if (value === null || typeof value !== "object") {
			return value;
		}

		return Object.fromEntries(
			Object.entries(value).flatMap(([name, entry]) =>
				entry === undefined
					? []
					: [[diagnosticNames[name] ?? name, convert(entry)]],
			),
		) as JSONSerializable;
	};

	return convert(payload);
};

export const decodeTelemetryTable = (
	frameType: Frame,
	frame: flatbuffers.IUnpackableObject<unknown> | null,
): Record<string, JSONSerializable> => {
	const payload = plain(frame?.unpack(), "", frameType);

	if (
		payload === undefined ||
		payload === null ||
		typeof payload !== "object" ||
		Array.isArray(payload)
	) {
		throw new Error("telemetry envelope has no frame payload");
	}

	switch (frameType) {
		case Frame.MeasurementsFrame:
			return { measurements: payload.rows ?? [] };
		case Frame.TickFrame:
			return { tick: payload };
		case Frame.EquityFrame:
			return { equity: payload };
		case Frame.BalancesFrame:
			return {
				balances: Object.fromEntries(
					(Array.isArray(payload.balances) ? payload.balances : []).flatMap(
						(balance) => {
							if (
								balance === null ||
								typeof balance !== "object" ||
								Array.isArray(balance)
							) {
								return [];
							}

							return typeof balance.asset === "string"
								? [[balance.asset, balance.amount]]
								: [];
						},
					),
				) as JSONSerializable,
			};
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
		case Frame.FluidPhaseFrame:
			return {
				fluidPhase: {
					...payload,
					phaseScan: Array.isArray(payload.phaseScan)
						? payload.phaseScan.map((response) => {
								if (
									response === null ||
									typeof response !== "object" ||
									Array.isArray(response)
								) {
									throw new Error("phase response must be an object");
								}

								const outcome = response.outcome;

								return outcome !== null &&
									typeof outcome === "object" &&
									!Array.isArray(outcome)
									? {
											...response,
											outcome: { ...outcome, return: outcome.returnValue },
										}
									: response;
							})
						: [],
				},
			};
		case Frame.DiagnosticsFrame:
			return {
				diagnostics: diagnosticsPayload(
					payload as Record<string, JSONSerializable>,
				),
			};
		default:
			throw new Error(`unsupported telemetry schema tag ${frameType}`);
	}
};

export const decodeTelemetryFrame = (
	bytes: Uint8Array,
): Record<string, JSONSerializable> => {
	const byteBuffer = new flatbuffers.ByteBuffer(bytes);

	if (!Envelope.bufferHasIdentifier(byteBuffer)) {
		throw new Error("telemetry frame has no SYMM FlatBuffers identifier");
	}

	const envelope = Envelope.getRootAsEnvelope(byteBuffer);
	const frameType = envelope.frameType();
	const frame = unionToFrame(frameType, envelope.frame.bind(envelope));

	return decodeTelemetryTable(frameType, frame);
};

export const decodeTelemetryBatch = (
	batch: ArrayBuffer,
): Record<string, JSONSerializable>[] => {
	const byteBuffer = new flatbuffers.ByteBuffer(new Uint8Array(batch));

	if (!byteBuffer.__has_identifier("SYMB")) {
		throw new Error("telemetry batch has no SYMB FlatBuffers identifier");
	}

	const encoded = Batch.getRootAsBatch(byteBuffer);
	const frames: Record<string, JSONSerializable>[] = [];

	for (let index = 0; index < encoded.framesLength(); index += 1) {
		const entry = encoded.frames(index);

		if (entry === null) {
			throw new Error(`telemetry batch frame ${index} is missing`);
		}

		const frameType = entry.frameType();
		const frame = unionToFrame(frameType, entry.frame.bind(entry));

		if (frame === null) {
			throw new Error(`telemetry batch frame ${index} has no payload`);
		}

		frames.push(decodeTelemetryTable(frameType, frame));
	}

	if (frames.length === 0) {
		throw new Error("telemetry batch is truncated");
	}

	return frames;
};
