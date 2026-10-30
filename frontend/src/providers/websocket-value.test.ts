import { describe, expect, it } from "vitest";
import { atoms, signals } from "#/collections/app";
import { encodeValueWire as encodeValue } from "#/test/encode-value-wire";
import {
	parseOneValue,
	parseValues,
	parseWireValues,
	valueFromWire,
} from "#/types/decoder";

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

	it("parseWireValues matches parseValues for batched frames", () => {
		const v1 = encodeValue({ origin: "cvd", scope: "BTC/USD", tick: 1n });
		const v2 = encodeValue({
			origin: "training",
			scope: "BTC/USD",
			tick: 2n,
		});
		const batch = new Uint8Array(v1.byteLength + v2.byteLength);
		batch.set(v1, 0);
		batch.set(v2, v1.byteLength);

		const wire = parseWireValues(batch);
		expect(wire?.length).toBe(2);
		const values = wire?.map(valueFromWire);
		expect(values?.[0].origin).toBe("cvd");
		expect(values?.[1].origin).toBe("training");
		expect(parseValues(batch)?.[1].tick).toBe(2n);
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
		// Production ingest only copies tick from workspace/aggregate (see advanceMarketTick).
		// Here we set from the training frame header to assert decode/routing only.
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
