import { describe, expect, it } from "vitest";
import {
	cashAtom,
	decisionsAtom,
	equityAtom,
	signals,
	symbolsAtom,
	tickCountAtom,
	unrealizedAtom,
} from "#/collections/app";
import {
	dispatchValue,
	parseOneValue,
	parseValues,
} from "./websocket";

function encodeValue({
	id = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]),
	tick = 100n,
	seqIdx = 1n,
	timestamp = 1718028000000000000n,
	origin = "training",
	role = "frame",
	scope = "BTC/USD",
	layers = [] as Array<{
		index: string;
		vector?: number[];
		json?: Record<string, unknown>;
	}>,
}) {
	const textEncoder = new TextEncoder();
	const originBytes = textEncoder.encode(origin);
	const roleBytes = textEncoder.encode(role);
	const scopeBytes = textEncoder.encode(scope);

	let size =
		40 + 1 + originBytes.length + 1 + roleBytes.length + 1 + scopeBytes.length + 2;
	const encodedLayers: Array<{ idxBytes: Uint8Array; payloadBytes: Uint8Array }> = [];

	for (const l of layers) {
		const idxBytes = textEncoder.encode(l.index);
		let payloadBytes: Uint8Array;
		if (l.vector) {
			payloadBytes = new Uint8Array(l.vector.length * 8);
			const dv = new DataView(payloadBytes.buffer);
			for (let i = 0; i < l.vector.length; i++) {
				dv.setFloat64(i * 8, l.vector[i], true);
			}
		} else if (l.json) {
			payloadBytes = textEncoder.encode(JSON.stringify(l.json));
		} else {
			payloadBytes = new Uint8Array(0);
		}
		encodedLayers.push({ idxBytes, payloadBytes });
		size += 2 + idxBytes.length + 4 + payloadBytes.length;
	}

	const buf = new Uint8Array(size);
	const dv = new DataView(buf.buffer);

	buf.set(id, 0);
	dv.setBigUint64(16, tick, true);
	dv.setBigUint64(24, seqIdx, true);
	dv.setBigUint64(32, timestamp, true);

	let offset = 40;
	buf[offset++] = originBytes.length;
	buf.set(originBytes, offset);
	offset += originBytes.length;

	buf[offset++] = roleBytes.length;
	buf.set(roleBytes, offset);
	offset += roleBytes.length;

	buf[offset++] = scopeBytes.length;
	buf.set(scopeBytes, offset);
	offset += scopeBytes.length;

	dv.setUint16(offset, encodedLayers.length, true);
	offset += 2;

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

describe("Value Binary Decoder and Dispatcher", () => {
	it("parses single *data.Value binary wire frame", () => {
		const raw = encodeValue({
			origin: "cvd",
			role: "signal",
			scope: "ETH/USD",
			tick: 42n,
			seqIdx: 7n,
			timestamp: 1718028000000000000n,
			layers: [
				{
					index: "cvd/signal/ETH/USD/0000000000000000042/0000000000000000007.bin/delta/volume",
					vector: [12.5, 3400.0],
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
		expect(parsed?.layers.length).toBe(1);
		expect(parsed?.layers[0].index).toContain("delta/volume");
		expect(parsed?.layers[0].bytes.byteLength).toBe(16);
	});

	it("parses concatenated batches of *data.Value records", () => {
		const v1 = encodeValue({ origin: "cvd", scope: "BTC/USD", tick: 1n });
		const v2 = encodeValue({ origin: "training", scope: "BTC/USD", tick: 2n });

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
	});

	it("dispatches training frame into signals.training ring and updates decisionsAtom", () => {
		const raw = encodeValue({
			origin: "training",
			role: "frame",
			scope: "BTC/USD",
			tick: 105n,
			timestamp: 1718028000000000000n,
			layers: [
				{
					index: "training/frame/BTC/USD/0000000000000000105/0000000000000000001.bin/surprise/energy/action:enter/path:R01/R06",
					vector: [0.85, 0.42],
				},
			],
		});

		const parsed = parseOneValue(raw);
		expect(parsed).not.toBeNull();
		dispatchValue(parsed!);

		// Symbol observed
		expect(symbolsAtom.get()).toContain("BTC/USD");

		// Training signal ring updated
		const ring = signals.training.state["BTC/USD"];
		expect(ring).toBeDefined();
		const latest = ring.get(ring.getBufferLength() - 1);
		expect(latest).toBeDefined();
		expect(latest?.origin).toBe("training");
		expect(latest?.scope).toBe("BTC/USD");
		expect(latest?.tick).toBe(105n);

		// Metrics extracted from layer
		const surpriseMetric = latest?.metrics.find((m) => m.name === "surprise");
		expect(surpriseMetric?.raw).toBeCloseTo(0.85);
		const energyMetric = latest?.metrics.find((m) => m.name === "energy");
		expect(energyMetric?.raw).toBeCloseTo(0.42);

		// Provenance extracted
		const actionProv = latest?.provenance.find((p) => p.name === "decision_action");
		expect(actionProv?.value).toBe("enter");
		const pathProv = latest?.provenance.find((p) => p.name === "token_path");
		expect(pathProv?.value).toBe("R01/R06");

		// DecisionsAtom updated
		const decisions = decisionsAtom.get();
		const btcDecision = decisions.find((d) => d.symbol === "BTC/USD");
		expect(btcDecision).toBeDefined();
		expect(btcDecision?.action).toBe("enter");
		expect(btcDecision?.cause).toBe("R01/R06");
	});

	it("dispatches balance JSON frame and updates equity atoms", () => {
		const raw = encodeValue({
			origin: "broker",
			role: "balance",
			scope: "USD",
			layers: [
				{
					index: "broker/balance/USD",
					json: {
						cash: "10000.50",
						unrealized: "250.75",
						equity: "10251.25",
					},
				},
			],
		});

		const parsed = parseOneValue(raw);
		expect(parsed).not.toBeNull();
		dispatchValue(parsed!);

		expect(cashAtom.get()).toBe("10000.50");
		expect(unrealizedAtom.get()).toBe("250.75");
		expect(equityAtom.get()).toBe("10251.25");
	});

	it("dispatches spot trade frame and updates tickCountAtom", () => {
		const raw = encodeValue({
			origin: "spot:trade",
			role: "trade",
			scope: "SOL/USD",
			tick: 9999n,
		});

		const parsed = parseOneValue(raw);
		expect(parsed).not.toBeNull();
		dispatchValue(parsed!);

		expect(tickCountAtom.get()).toBe(9999);
	});
});
