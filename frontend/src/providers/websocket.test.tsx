// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { atoms, routeAtom } from "#/collections/app";
import { cognitionStore } from "#/collections/cognition";
import { sequiturStore } from "#/collections/sequitur";
import { sequiturFrame } from "#/collections/sequitur-fixture";
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
	it("ingests sequitur frames even though the origin has no signals store", () => {
		const layer = new TextEncoder().encode(JSON.stringify(sequiturFrame()));

		ingest([
			new Value({
				origin: "sequitur",
				role: "grammar",
				scope: "BTC/USD",
				prefix: "sequitur",
				index: ["sequitur/grammar/BTC-USD"],
				layers: [layer],
				tick: 41n,
			}),
		]);

		expect(sequiturStore.state["BTC/USD"]?.latest.tick).toBe(41);
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

	it("still drops unbound origins rather than routing them into nothing", () => {
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

		const probe = window.__kilo_probe;
		expect(probe.missed).toContainEqual(
			expect.objectContaining({ origin: "kraken" }),
		);
	});
});
