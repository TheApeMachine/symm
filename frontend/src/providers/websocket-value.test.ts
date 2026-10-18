import { describe, expect, it, vi } from "vitest";
import {
	cashAtom,
	decisionsAtom,
	deskPositionsAtom,
	equityAtom,
	positionsAtom,
	resonanceStore,
	signals,
	symbolsAtom,
	tickCountAtom,
	unrealizedAtom,
} from "#/collections/app";
import { cognitionStore } from "#/collections/cognition";
import {
	beginHindsight,
	HINDSIGHT_TICK_KEYS,
	hindsightStore,
} from "#/collections/hindsight";
import { PAPER_KEYS, paperStore } from "#/collections/paper";
import { sequiturStore } from "#/collections/sequitur";
import { sequiturFrame } from "#/collections/sequitur-fixture";
import { parseResonanceData } from "#/components/charts/prediction";
import {
	dispatchValue,
	parseOneValue,
	parseSignalIndex,
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
		json?: unknown;
		raw?: Uint8Array;
	}>,
}) {
	const textEncoder = new TextEncoder();
	const originBytes = textEncoder.encode(origin);
	const roleBytes = textEncoder.encode(role);
	const scopeBytes = textEncoder.encode(scope);

	let size =
		48 +
		1 +
		originBytes.length +
		1 +
		roleBytes.length +
		1 +
		scopeBytes.length +
		2;
	const encodedLayers: Array<{
		idxBytes: Uint8Array;
		payloadBytes: Uint8Array;
	}> = [];

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
		} else if (l.raw) {
			payloadBytes = l.raw;
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
	dv.setBigUint64(40, 0n, true);

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
					index:
						"cvd/signal/ETH/USD/0000000000000000042/0000000000000000007.bin/delta/volume",
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
					index:
						"training/frame/BTC/USD/0000000000000000105/0000000000000000001.bin/surprise/energy/action:enter/path:R01/R06",
					vector: [0.85, 0.42],
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

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
		const actionProv = latest?.provenance.find(
			(p) => p.name === "decision_action",
		);
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

	it("routes a tick's signal layer by its canonical index (data.SignalIndex)", () => {
		const raw = encodeValue({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			tick: 9n,
			layers: [
				{
					index:
						"signal/cvd/midpoint_log_return/signed_net_fraction/symbol:ETH/USD",
					vector: [0.001, -0.5],
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		const ring = signals.cvd.state["ETH/USD"];
		expect(ring).toBeDefined();
		const latest = ring.get(ring.getBufferLength() - 1);
		expect(latest?.origin).toBe("cvd");
		expect(latest?.scope).toBe("ETH/USD");
		expect(
			latest?.metrics.find((m) => m.name === "signed_net_fraction")?.raw,
		).toBeCloseTo(-0.5);
	});

	it("routes a prefixed gob signal layer and the gob balance", () => {
		const raw = encodeValue({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			tick: 11n,
			layers: [
				{
					index:
						"workspace/tick/aggregate/signal/cvd/midpoint_log_return/signed_net_fraction/symbol:BTC/USD",
					raw: new Uint8Array([
						11, 127, 2, 1, 2, 255, 128, 0, 1, 8, 0, 0, 16, 255, 128, 0, 2,
						248, 252, 169, 241, 210, 77, 98, 80, 63, 254, 224, 191,
					]),
				},
				{
					index: "workspace/tick/aggregate/balance",
					raw: new Uint8Array([
						46, 10, 0, 43, 123, 34, 100, 97, 116, 97, 34, 58, 91, 123, 34,
						97, 115, 115, 101, 116, 34, 58, 34, 85, 83, 68, 34, 44, 34, 98,
						97, 108, 97, 110, 99, 101, 34, 58, 34, 49, 50, 46, 53, 34, 125,
						93, 125,
					]),
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		const ring = signals.cvd.state["BTC/USD"];
		expect(ring).toBeDefined();
		const latest = ring.get(ring.getBufferLength() - 1);
		expect(
			latest?.metrics.find((m) => m.name === "signed_net_fraction")?.raw,
		).toBeCloseTo(-0.5);
		expect(cashAtom.get()).toBe("12.5");
	});

	it("parseSignalIndex rejects a signal index without a symbol tag", () => {
		expect(
			parseSignalIndex("signal/cvd/gross_notional/symbol:BTC/USD"),
		).toEqual({
			source: "cvd",
			symbol: "BTC/USD",
			labels: ["gross_notional"],
		});
		expect(
			parseSignalIndex(
				"signal/cvd/gross_notional/ref:x/symbol:y/symbol:SOL/USD",
			),
		).toEqual({
			source: "cvd",
			symbol: "SOL/USD",
			labels: ["gross_notional", "ref:x", "symbol:y"],
		});
		expect(parseSignalIndex("signal/cvd/gross_notional")).toBeNull();
		expect(parseSignalIndex("price/BTC/USD")).toBeNull();
	});

	it("dispatches the balance layer into the cash atom", () => {
		const raw = encodeValue({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			layers: [
				{
					index: "balance",
					json: {
						channel: "balances",
						data: [{ asset: "USD", balance: "10000.50" }],
					},
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		expect(cashAtom.get()).toBe("10000.50");
	});

	it("dispatches unrealized and equity from the balance layer", () => {
		const raw = encodeValue({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			layers: [
				{
					index: "balance",
					json: {
						data: [{ asset: "USD", balance: "120" }],
						n: "-4.5",
						e: "195.5",
					},
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		expect(cashAtom.get()).toBe("120");
		expect(unrealizedAtom.get()).toBe("-4.5");
		expect(equityAtom.get()).toBe("195.5");
	});

	it("dispatches spot trade frame and updates tickCountAtom", () => {
		const raw = encodeValue({
			origin: "spot:trade",
			role: "trade",
			scope: "SOL/USD",
			tick: 9999n,
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		expect(tickCountAtom.get()).toBe(9999);
	});

	it("routes an aggregate tick's layers to the tick symbol's consumers", () => {
		const raw = encodeValue({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			tick: 77n,
			layers: [
				{
					index: "spot:trade/update",
					json: { channel: "trade", data: [{ symbol: "ETH/USD", price: 1 }] },
				},
				{ index: "price/ETH/USD", json: { symbol: "ETH/USD" } },
				{
					index: "positions",
					json: [{ symbol: "ETH/USD", status: "open", entry_price: "2" }],
				},
				{
					index:
						"signal/hawkes/arrival_rate:buy/arrival_rate:sell/symbol:ETH/USD",
					vector: [1.5, 0.5],
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		expect(symbolsAtom.get()).toContain("ETH/USD");
		expect(tickCountAtom.get()).toBe(77);
		expect(positionsAtom.get()[0]?.holding?.symbol).toBe("ETH/USD");

		const ring = signals.hawkes.state["ETH/USD"];
		const latest = ring?.get(ring.getBufferLength() - 1);
		expect(latest?.origin).toBe("hawkes");
		expect(latest?.metric("arrival_rate:buy")).toBeCloseTo(1.5);
		expect(latest?.metric("arrival_rate:sell")).toBeCloseTo(0.5);
	});

	it("reads a position's entry and fee from its fills and the mark from the price layer", () => {
		const raw = encodeValue({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			layers: [
				{
					index: "price/BTC/USD",
					json: { symbol: "BTC/USD", bid: "101.5", ask: "102" },
				},
				{
					index: "positions",
					json: [
						{
							id: "p1",
							symbol: "BTC/USD",
							opened_at: "2026-10-18T09:00:00Z",
							closed_at: null,
							qty: "0.01",
							virtual: false,
							entry_orders: [
								{ type: "buy", ordertype: "market", volume: "0.01" },
							],
							fills: [
								{
									side: "buy",
									avg_price: "100",
									last_price: "100",
									fee_usd_equiv: "0.08",
								},
							],
						},
					],
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		expect(deskPositionsAtom.get()[0]).toMatchObject({
			symbol: "BTC/USD",
			qty: "0.01",
			entryPrice: "100",
			entryFee: "0.08",
			mark: "101.5",
		});
	});

	it("reads exit room as visible bid size divided by the lot", () => {
		const raw = encodeValue({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			layers: [
				{
					index: "book/SENT/USD",
					json: {
						bids: {
							levels: {
								a: { quantity: "400" },
								b: { quantity: "121.12387102" },
							},
						},
					},
				},
				{
					index: "positions",
					json: [
						{
							id: "sent",
							symbol: "SENT/USD",
							opened_at: "2026-10-18T09:00:00Z",
							closed_at: null,
							qty: "521.12387102",
							virtual: false,
						},
					],
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		expect(deskPositionsAtom.get()[0]?.exitRoom).toBeCloseTo(1);
	});

	it("routes each symbol's signal layer of a multi-symbol tick to that symbol", () => {
		const raw = encodeValue({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			tick: 78n,
			layers: [
				{
					index: "spot:trade/update",
					json: {
						channel: "trade",
						data: [{ symbol: "ETH/USD" }, { symbol: "SOL/USD" }],
					},
				},
				{ index: "price/ETH/USD", json: { symbol: "ETH/USD" } },
				{
					index:
						"signal/hawkes/arrival_rate:buy/arrival_rate:sell/symbol:ETH/USD",
					vector: [1, 2],
				},
				{
					index:
						"signal/hawkes/arrival_rate:buy/arrival_rate:sell/symbol:SOL/USD",
					vector: [3, 4],
				},
			],
		});

		const parsed = parseOneValue(raw);
		if (!parsed) {
			throw new Error("fixture did not parse");
		}
		dispatchValue(parsed);

		const eth = signals.hawkes.state["ETH/USD"];
		const sol = signals.hawkes.state["SOL/USD"];
		expect(
			eth?.get(eth.getBufferLength() - 1)?.metric("arrival_rate:buy"),
		).toBeCloseTo(1);
		expect(
			sol?.get(sol.getBufferLength() - 1)?.metric("arrival_rate:buy"),
		).toBeCloseTo(3);
		expect(
			sol?.get(sol.getBufferLength() - 1)?.metric("arrival_rate:sell"),
		).toBeCloseTo(4);
	});
});

describe("dispatchValue: sequitur grammar frames", () => {
	it("routes sequitur/grammar/<symbol> to the sequitur feed, not the tick path", () => {
		sequiturStore.setState(() => ({}));
		const before = tickCountAtom.get();
		const parsed = parseOneValue(
			encodeValue({
				tick: 9999n,
				origin: "sequitur",
				role: "grammar",
				scope: "BTC/USD",
				layers: [{ index: "grammar", json: sequiturFrame() }],
			}),
		);
		expect(parsed).not.toBeNull();
		if (!parsed) return;

		dispatchValue(parsed);

		expect(sequiturStore.state["BTC/USD"]?.latest.path).toBe("R02/R04");
		expect(tickCountAtom.get()).toBe(before);
	});

	it("reports a sequitur Value without its grammar layer", () => {
		sequiturStore.setState(() => ({}));
		const error = vi.spyOn(console, "error").mockImplementation(() => {});
		const parsed = parseOneValue(
			encodeValue({
				origin: "sequitur",
				role: "grammar",
				layers: [{ index: "other", json: {} }],
			}),
		);
		if (!parsed) throw new Error("fixture did not parse");

		dispatchValue(parsed);

		expect(error).toHaveBeenCalled();
		expect(sequiturStore.state).toEqual({});
		error.mockRestore();
	});
});

describe("dispatchValue: paper-trade records", () => {
	const index =
		"paper/enter/BTC/USD/1792187257188073099/0000000000000000001.bin/" +
		`${PAPER_KEYS.join("/")}/path:R01/R04/policy_epoch:1792187257188073000`;

	it("routes paper/<action>/<symbol> to the paper store with every key, its path and policy epoch", () => {
		paperStore.setState(() => []);
		const parsed = parseOneValue(
			encodeValue({
				tick: 1n,
				origin: "paper",
				role: "enter",
				scope: "BTC/USD",
				layers: [{ index, vector: PAPER_KEYS.map((_, idx) => idx + 0.5) }],
			}),
		);
		if (!parsed) throw new Error("fixture did not parse");

		dispatchValue(parsed);

		const [record] = paperStore.state;
		expect(paperStore.state).toHaveLength(1);
		expect(record.action).toBe("enter");
		expect(record.symbol).toBe("BTC/USD");
		expect(record.path).toBe("R01/R04");
		expect(record.policyEpoch).toBe("1792187257188073000");
		expect(record.fields.depth).toBe(0.5);
		expect(record.fields.net_return).toBe(14.5);
	});

	it("reports a paper record whose layer misses a key", () => {
		paperStore.setState(() => []);
		const error = vi.spyOn(console, "error").mockImplementation(() => {});
		const parsed = parseOneValue(
			encodeValue({
				origin: "paper",
				role: "exit",
				layers: [
					{
						index: index.replace("/fees", ""),
						vector: PAPER_KEYS.slice(1).map(() => 1),
					},
				],
			}),
		);
		if (!parsed) throw new Error("fixture did not parse");

		dispatchValue(parsed);

		expect(error).toHaveBeenCalled();
		expect(paperStore.state).toEqual([]);
		error.mockRestore();
	});
});

describe("dispatchValue: hindsight frames", () => {
	it("routes hindsight/<role>/<symbol> to the replay store, not the live tick path", async () => {
		const run = "1792187257188073000";
		beginHindsight(run, "BTC/USD");
		const before = tickCountAtom.get();
		const index =
			"hindsight/tick/BTC/USD/1792187257188073000/0000000000000000005.bin/" +
			HINDSIGHT_TICK_KEYS.join("/");
		const parsed = parseOneValue(
			encodeValue({
				tick: 5n,
				origin: "hindsight",
				role: "tick",
				scope: "BTC/USD",
				layers: [{ index, vector: [99.5, 100.5, 100, 0.25, 1] }],
			}),
		);
		if (!parsed) throw new Error("fixture did not parse");
		parsed.epoch = BigInt(run);

		dispatchValue(parsed);
		// Replay frames are applied to the store in batches.
		await new Promise((resolve) => setTimeout(resolve, 80));

		expect(tickCountAtom.get()).toBe(before);
		expect(
			hindsightStore.state.stream?.ticks.map((tick) => [
				tick.tick,
				tick.fields.ask,
			]),
		).toEqual([[5, 100.5]]);
		hindsightStore.setState(() => ({ index: null, stream: null }));
	});
});

describe("dispatchValue: resonance readings", () => {
	it("routes resonance/reading/<symbol> into the resonance store the prediction chart reads", () => {
		resonanceStore.setState(() => ({}));
		const prefix =
			"resonance/reading/BTC/USD/0000000000000000007/0000000000000000040.bin";
		const parsed = parseOneValue(
			encodeValue({
				tick: 40n,
				origin: "resonance",
				role: "reading",
				scope: "BTC/USD",
				layers: [
					{ index: `${prefix}/energy`, vector: [0.12] },
					{ index: `${prefix}/surprise`, vector: [0.45] },
					{ index: `${prefix}/latent_0`, vector: [0.3] },
					{ index: `${prefix}/layer_0_state_0`, vector: [1] },
					{ index: `${prefix}/layer_0_prediction_0`, vector: [0.5] },
				],
			}),
		);
		if (!parsed) throw new Error("fixture did not parse");

		dispatchValue(parsed);

		const ring = resonanceStore.state["BTC/USD"];
		const last = ring?.getLast();
		expect(last?.origin).toBe("resonance");
		expect(last?.scope).toBe("BTC/USD");
		expect(last?.tick).toBe(40n);

		const artifact = parseResonanceData(last);
		expect(artifact?.energy).toBeCloseTo(0.12);
		expect(artifact?.surprise).toBeCloseTo(0.45);
		expect(artifact?.latent).toEqual([0.3]);
		expect(artifact?.layers?.[0]?.state).toEqual([1]);
		expect(artifact?.layers?.[0]?.prediction).toEqual([0.5]);
		resonanceStore.setState(() => ({}));
	});
});

describe("dispatchValue: train/regions on ticks", () => {
	it("updates cognition matches from train/regions layers", () => {
		cognitionStore.setState(() => ({
			epoch: "0",
			policyKeys: [],
			matches: {},
			problem: null,
			tree: null,
		}));
		const parsed = parseOneValue(
			encodeValue({
				origin: "workspace",
				role: "tick",
				scope: "aggregate",
				tick: 9n,
				layers: [
					{
						index:
							"workspace/tick/aggregate/1/0000000000000000009.bin/train/regions/BTC/USD/R01/R02",
						vector: [0.8, 0.6],
					},
				],
			}),
		);
		if (!parsed) throw new Error("fixture did not parse");
		dispatchValue(parsed);
		expect(cognitionStore.state.matches["BTC/USD"]?.tokens).toEqual([
			"R01",
			"R02",
		]);
	});
});

describe("dispatchValue: cognition frames", () => {
	it("routes cognition/tree and cognition/match to the cognition store", () => {
		cognitionStore.setState(() => ({
			epoch: "0",
			policyKeys: [],
			matches: {},
			problem: null,
			tree: null,
		}));
		const before = tickCountAtom.get();
		const send = (role: string, scope: string, json: unknown) => {
			const parsed = parseOneValue(
				encodeValue({
					origin: "cognition",
					role,
					scope,
					layers: [{ index: "cognition", json }],
				}),
			);
			if (!parsed) throw new Error("fixture did not parse");
			dispatchValue(parsed);
		};

		send("tree", "all", {
			epoch: "7",
			paths: 0,
			trading: false,
			gate: null,
			keys: [],
			branches: [],
			problem: null,
		});
		send("match", "BTC/USD", {
			symbol: "BTC/USD",
			tick: 3,
			tokens: ["R01"],
			stored: false,
			match: "",
			depth: 0,
			action: "",
		});

		expect(cognitionStore.state.tree?.trading).toBe(false);
		expect(cognitionStore.state.matches["BTC/USD"]?.tokens).toEqual(["R01"]);
		expect(tickCountAtom.get()).toBe(before);
	});
});
