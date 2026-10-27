// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { atoms, routeAtom, signals } from "#/collections/app";
import { cognitionStore } from "#/collections/cognition";
import { sequiturStore } from "#/collections/sequitur";
import { Value } from "#/types/value";
import { ingest, WsFeed } from "./websocket";

class FeedWorker {
	static latest: FeedWorker;
	postMessage = vi.fn();
	addEventListener = vi.fn();
	terminate = vi.fn();

	constructor() {
		FeedWorker.latest = this;
	}
}

afterEach(() => {
	cleanup();
	routeAtom.set("dashboard");
	atoms.tickCount.set(0);
	sequiturStore.setState(() => ({}));
	cognitionStore.setState(() => ({
		epoch: "0",
		policyKeys: [],
		thresholds: {},
		matches: {},
		problem: null,
		tree: null,
	}));
	vi.unstubAllGlobals();
});

describe("WsFeed", () => {
	it("seeds the current route before connecting and forwards subsequent navigation", () => {
		vi.stubGlobal("Worker", FeedWorker);
		routeAtom.set("xray");
		render(<WsFeed />);
		const worker = FeedWorker.latest;
		expect(worker.postMessage.mock.calls[0][0]).toEqual({
			type: "ROUTE",
			route: "xray",
		});
		expect(worker.postMessage.mock.calls[1][0]).toEqual({
			type: "CONNECT",
			url: "ws://127.0.0.1:8765/ws",
		});
		act(() => routeAtom.set("fluid"));
		expect(worker.postMessage).toHaveBeenLastCalledWith({
			type: "ROUTE",
			route: "fluid",
		});
	});
});

describe("ingest", () => {
	it("ingests training step values into the sequitur store", () => {
		const prefix = "training/step/BTC/USD/1/41";
		const enc = (payload: unknown) =>
			new TextEncoder().encode(JSON.stringify(payload));

		ingest([
			new Value({
				origin: "training",
				role: "step",
				scope: "BTC/USD",
				prefix,
				index: [
					`${prefix}/reported`,
					`${prefix}/blocked`,
					`${prefix}/gate`,
					`${prefix}/fired`,
					`${prefix}/active`,
					`${prefix}/midpoint`,
					`${prefix}/grammar/events`,
					`${prefix}/grammar/rules`,
					`${prefix}/grammar/start`,
					`${prefix}/grammar/rawLength`,
					`${prefix}/grammar/grammarSymbols`,
					`${prefix}/grammar/completed`,
				],
				layers: [
					enc("enter"),
					enc(false),
					enc({ surprise: 0.42, z: null, warm: true, passed: true }),
					enc(["R02", "R04"]),
					enc([
						{ key: 2, brightness: 0.8 },
						{ key: 4, brightness: 0.6 },
					]),
					enc(0.45),
					enc([
						{
							kind: "append",
							token: "R02",
							rule: 0,
							into: 0,
							from: 0,
							digram: ["", ""],
						},
					]),
					enc([]),
					enc([]),
					enc(5),
					enc(5),
					enc(""),
				],
				tick: 41n,
				epoch: 1n,
			}),
		]);

		expect(sequiturStore.state["BTC/USD"]?.latest.tick).toBe(41);
		expect(sequiturStore.state["BTC/USD"]?.latest.policy.action).toBe(
			"enter",
		);
		expect(atoms.tickCount.get()).toBe(0);
	});

	it("keeps a balance frame on the account atom", () => {
		const layer = new TextEncoder().encode(
			JSON.stringify({ cash: "12.50", equity: "40.00", unrealized: "-1.25" }),
		);
		const frame = new Value({
			origin: "balance",
			role: "update",
			scope: "all",
			prefix: "balance/update/all",
			index: ["balance/update/all/balance"],
			layers: [layer],
			tick: 9n,
		});

		ingest([frame]);

		expect(atoms.balance.get()).toBe(frame);
		expect(atoms.tickCount.get()).toBe(0);
	});

	it("advances the trade counter only on workspace aggregate ticks", () => {
		ingest([
			new Value({
				origin: "workspace",
				role: "tick",
				scope: "aggregate",
				prefix: "workspace/tick/aggregate",
				index: ["workspace/tick/aggregate/spot:trade"],
				layers: [new TextEncoder().encode("[]")],
				tick: 100n,
			}),
			new Value({
				origin: "manifold",
				role: "parallel",
				scope: "BTC-USD",
				prefix: "manifold/parallel/BTC-USD",
				index: [],
				layers: [],
				tick: 95n,
			}),
		]);

		expect(atoms.tickCount.get()).toBe(100);
	});

	it("uses each frame tick header, not one step per ingest call", () => {
		ingest([
			new Value({
				origin: "workspace",
				role: "tick",
				scope: "aggregate",
				prefix: "workspace/tick/aggregate",
				index: [],
				layers: [],
				tick: 40n,
			}),
			new Value({
				origin: "workspace",
				role: "tick",
				scope: "aggregate",
				prefix: "workspace/tick/aggregate",
				index: [],
				layers: [],
				tick: 512n,
			}),
		]);

		expect(atoms.tickCount.get()).toBe(512);
	});

	it("ingests cognition match frames", () => {
		const match = {
			symbol: "BTC/USD",
			tick: 41,
			tokens: ["R02"],
			stored: false,
			match: "",
			depth: 1,
			action: "noop",
		};

		ingest([
			new Value({
				origin: "cognition",
				role: "match",
				scope: "BTC/USD",
				prefix: "cognition",
				index: ["cognition/match/BTC-USD"],
				layers: [new TextEncoder().encode(JSON.stringify(match))],
				tick: 41n,
			}),
		]);

		expect(cognitionStore.state.matches["BTC/USD"]?.action).toBe("noop");
	});

	it("ignores origins that are not in valueAtoms or signals", () => {
		ingest([
			new Value({
				origin: "kraken",
				role: "ack",
				scope: "BTC/USD",
				prefix: "kraken",
				index: [],
				layers: [],
			}),
		]);

		expect(signals.kraken).toBeUndefined();
	});

	it("keeps the richer workspace aggregate when the same tick is fanned out twice", () => {
		signals.workspace.setState(() => ({}));

		const prefix = "workspace/tick/aggregate/1/99";
		const rich = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix,
			tick: 99n,
			index: [
				`${prefix}/cvd/BTC/USD/midpoint_log_return`,
				`${prefix}/train/regions/BTC/USD`,
			],
			layers: [
				new TextEncoder().encode(JSON.stringify([0.42])),
				new TextEncoder().encode(
					JSON.stringify([{ symbol: "BTC/USD", key: 1, brightness: 0.8 }]),
				),
			],
		});
		const sparse = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix,
			tick: 99n,
			index: [`${prefix}/spot:trade`],
			layers: [new TextEncoder().encode(JSON.stringify([]))],
		});

		ingest([rich]);
		ingest([sparse]);

		const aggregate = signals.workspace.state.aggregate;
		expect(aggregate?.index.length).toBe(3);
		expect(aggregate?.index.some((idx) => idx.includes("cvd/BTC/USD"))).toBe(
			true,
		);
		expect(aggregate?.index.some((idx) => idx.includes("spot:trade"))).toBe(
			true,
		);
	});

	it("replaces the aggregate on a new tick without carrying prior layers", () => {
		signals.workspace.setState(() => ({}));

		const priorPrefix = "workspace/tick/aggregate/0/10";
		const nextPrefix = "workspace/tick/aggregate/0/11";
		ingest([
			new Value({
				origin: "workspace",
				role: "tick",
				scope: "aggregate",
				prefix: priorPrefix,
				tick: 10n,
				index: [`${priorPrefix}/cvd/BTC/USD/signed_net_fraction`],
				layers: [new TextEncoder().encode(JSON.stringify([-0.2]))],
			}),
		]);
		ingest([
			new Value({
				origin: "workspace",
				role: "tick",
				scope: "aggregate",
				prefix: nextPrefix,
				tick: 11n,
				index: [`${nextPrefix}/spot:trade`],
				layers: [new TextEncoder().encode(JSON.stringify([]))],
			}),
		]);

		const aggregate = signals.workspace.state.aggregate;
		expect(aggregate?.tick).toBe(11n);
		expect(aggregate?.index.length).toBe(1);
		expect(aggregate?.index[0]).toContain("spot:trade");
	});
});
