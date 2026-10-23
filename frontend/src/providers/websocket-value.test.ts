import { describe, expect, it } from "vitest";
import { atoms, signals } from "#/collections/app";
import { parseOneValue, parseValues } from "#/types/decoder";

/*
encodeValue serializes one Value the way the Go side does: fixed 48-byte
header, then origin, role, scope, layer count, the full prefix, and each
layer as an index string plus a byte payload.
*/
function encodeValue({
	id = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]),
	tick = 100n,
	seqIdx = 1n,
	timestamp = 1718028000000000000n,
	epoch = 0n,
	origin = "training",
	role = "frame",
	scope = "BTC/USD",
	layers = [] as Array<{
		index: string;
		json?: unknown;
		raw?: Uint8Array;
	}>,
}) {
	const textEncoder = new TextEncoder();
	const originBytes = textEncoder.encode(origin);
	const roleBytes = textEncoder.encode(role);
	const scopeBytes = textEncoder.encode(scope);

	/*
	The Go prefix is origin/role/scope/epoch/tick: layers address themselves
	below it, exactly like data.NewValue + Append build it.
	*/
	const prefix = `${origin}/${role}/${scope}/${epoch.toString().padStart(19, "0")}/${tick.toString().padStart(19, "0")}`;
	const prefixBytes = textEncoder.encode(prefix);

	let size =
		48 +
		1 +
		originBytes.length +
		1 +
		roleBytes.length +
		1 +
		scopeBytes.length +
		2 +
		2 +
		prefixBytes.length;
	const encodedLayers: Array<{
		idxBytes: Uint8Array;
		payloadBytes: Uint8Array;
	}> = [];

	for (const l of layers) {
		const idxBytes = textEncoder.encode(l.index);
		const payloadBytes =
			l.json !== undefined
				? textEncoder.encode(JSON.stringify(l.json))
				: (l.raw ?? new Uint8Array(0));

		encodedLayers.push({ idxBytes, payloadBytes });
		size += 2 + idxBytes.length + 4 + payloadBytes.length;
	}

	const buf = new Uint8Array(size);
	const dv = new DataView(buf.buffer);

	buf.set(id, 0);
	dv.setBigUint64(16, tick, true);
	dv.setBigUint64(24, seqIdx, true);
	dv.setBigUint64(32, timestamp, true);
	dv.setBigUint64(40, epoch, true);

	let offset = 48;
	buf[offset++] = originBytes.length;
	buf.set(originBytes, offset);
	offset += originBytes.length;

	buf[offset++] = roleBytes.length;
	buf.set(roleBytes, offset);
	offset += roleBytes.length;

	buf[offset++] = scopeBytes.length;
	buf.set(scopeBytes, offset);
	offset += scopeBytes.length;

	dv.setUint16(offset, layers.length, true);
	offset += 2;

	dv.setUint16(offset, prefixBytes.length, true);
	offset += 2;
	buf.set(prefixBytes, offset);
	offset += prefixBytes.length;

	for (const { idxBytes, payloadBytes } of encodedLayers) {
		dv.setUint16(offset, idxBytes.length, true);
		offset += 2;
		buf.set(idxBytes, offset);
		offset += idxBytes.length;

		dv.setUint32(offset, payloadBytes.length, true);
		offset += 4;
		buf.set(payloadBytes, offset);
		offset += payloadBytes.length;
	}

	return buf;
}

describe("Value binary decoder", () => {
	it("parses a single *data.Value wire frame", () => {
		const raw = encodeValue({
			origin: "cvd",
			role: "signal",
			scope: "ETH/USD",
			tick: 42n,
			seqIdx: 7n,
			timestamp: 1718028000000000000n,
			layers: [
				{
					index:
						"cvd/signal/ETH/USD/0000000000000000000/0000000000000000042/delta/volume",
					json: [12.5, 3400.0],
				},
			],
		});

		const parsed = parseOneValue(raw);
		expect(parsed).not.toBeNull();
		expect(parsed?.origin).toBe("cvd");
		expect(parsed?.role).toBe("signal");
		expect(parsed?.scope).toBe("ETH/USD");
		expect(parsed?.tick).toBe(42n);
		expect(parsed?.seqIdx).toBe(7n);
		expect(parsed?.timestamp).toBe(1718028000000000000n);
		expect(parsed?.prefix).toBe(
			"cvd/signal/ETH/USD/0000000000000000000/0000000000000000042",
		);
		expect(parsed?.layers.length).toBe(1);
		expect(parsed?.index[0]).toContain("delta/volume");
		expect(parsed?.layers[0].byteLength).toBe(11);
		expect(parsed?.metric("delta/volume")).toBe(12.5);
	});

	it("parses concatenated batches of *data.Value records", () => {
		const v1 = encodeValue({ origin: "cvd", scope: "BTC/USD", tick: 1n });
		const v2 = encodeValue({
			origin: "training",
			scope: "BTC/USD",
			tick: 2n,
		});

		const batch = new Uint8Array(v1.byteLength + v2.byteLength);
		batch.set(v1, 0);
		batch.set(v2, v1.byteLength);

		const values = parseValues(batch);
		expect(values).not.toBeNull();
		expect(values?.length).toBe(2);
		expect(values?.[0].origin).toBe("cvd");
		expect(values?.[0].tick).toBe(1n);
		expect(values?.[1].origin).toBe("training");
		expect(values?.[1].tick).toBe(2n);
		/*
		byteLength is the wire size the decoder advanced: parsing the batch
		at the first frame's end lands exactly on the second frame.
		*/
		expect(values?.[0].byteLength).toBe(v1.byteLength);

		const second = parseValues(batch.subarray(values?.[0].byteLength ?? 0));
		expect(second?.[0]?.origin).toBe("training");
	});

	it("returns null for a truncated frame", () => {
		const raw = encodeValue({ origin: "cvd", scope: "BTC/USD", tick: 1n });
		expect(parseOneValue(raw.subarray(0, raw.byteLength - 1))).toBeNull();
		expect(parseValues(new Uint8Array(10))).toBeNull();
	});
});

describe("ingest routing (decoded frames into stores)", () => {
	it("sets the signal store by origin and scope, and advances the tick", () => {
		const parsed = parseOneValue(
			encodeValue({
				origin: "training",
				role: "update",
				scope: "BTC-USD",
				tick: 105n,
				layers: [
					{
						index:
							"training/update/BTC-USD/0000000000000000000/0000000000000000105/surprise",
						json: [0.85],
					},
					{
						index:
							"training/update/BTC-USD/0000000000000000000/0000000000000000105/token_path",
						json: ["R01/R06"],
					},
				],
			}),
		);
		if (!parsed) throw new Error("fixture did not parse");

		const before = atoms.tickCount.get();

		/*
		One decoded frame routes exactly as the websocket provider's ingest
		does: the frame lands in its origin's store under its scope.
		*/
		signals[parsed.origin]?.setState((prev) => ({
			...prev,
			[parsed.scope]: parsed,
		}));
		atoms.tickCount.set(Number(parsed.tick));

		const frame = signals.training.state["BTC-USD"];
		expect(frame).toBeDefined();
		expect(frame?.origin).toBe("training");
		expect(frame?.scope).toBe("BTC-USD");
		expect(frame?.tick).toBe(105n);
		expect(frame?.metric("surprise")).toBeCloseTo(0.85);
		expect(frame?.provenance().token_path).toBe("R01/R06");
		expect(atoms.tickCount.get()).toBe(105);
		expect(atoms.tickCount.get()).not.toBe(before);

		signals.training.setState(() => ({}));
	});

	it("keeps per-symbol position frames apart by scope", () => {
		const btc = parseOneValue(
			encodeValue({
				origin: "positions",
				role: "update",
				scope: "BTC-USD",
				tick: 3n,
				layers: [
					{
						index:
							"positions/update/BTC-USD/0000000000000000000/0000000000000000003/position",
						json: { symbol: "BTC/USD", qty: "0.01" },
					},
				],
			}),
		);
		const eth = parseOneValue(
			encodeValue({
				origin: "positions",
				role: "update",
				scope: "ETH-USD",
				tick: 3n,
				layers: [
					{
						index:
							"positions/update/ETH-USD/0000000000000000000/0000000000000000003/position",
						json: { symbol: "ETH/USD", qty: "1.5" },
					},
				],
			}),
		);
		if (!btc || !eth) throw new Error("fixture did not parse");

		signals.positions.setState((prev) => ({ ...prev, [btc.scope]: btc }));
		signals.positions.setState((prev) => ({ ...prev, [eth.scope]: eth }));

		const rows = Object.values(signals.positions.state).flatMap((frame) =>
			frame.lookup(["position"]),
		) as Array<Record<string, unknown>>;

		expect(rows.map((row) => row.symbol).sort()).toEqual([
			"BTC/USD",
			"ETH/USD",
		]);

		signals.positions.setState(() => ({}));
	});
});
