import { beforeEach, describe, expect, it } from "vitest";
import { atoms } from "#/collections/app";
import {
	applyDiagPulse,
	applyDiagPulses,
	BURST_MS,
	diagBursts,
	isBursting,
} from "#/collections/diag-burst";
import { EMPTY_PIPELINE } from "#/collections/pipeline";

describe("applyDiagPulses", () => {
	beforeEach(() => {
		diagBursts.setState(() => ({}));
		atoms.pipeline.set({
			...EMPTY_PIPELINE,
			nodes: [
				{
					id: "trade",
					parent: "kraken",
					n: 1,
					ema_ns: 0,
					last_ns: 0,
					status: "ok",
				},
				{
					id: "ingress",
					n: 1,
					ema_ns: 0,
					last_ns: 0,
					status: "ok",
				},
			],
			hub: "ok",
		});
	});

	it("marks the node bursting and patches live metrics", () => {
		applyDiagPulse({ id: "trade", n: 9, last_ns: 1200, at: 1 });

		expect(isBursting(diagBursts.state, "trade")).toBe(true);
		expect(isBursting(diagBursts.state, "trade", Date.now() + BURST_MS + 1)).toBe(
			false,
		);

		const trade = atoms.pipeline
			.get()
			.nodes.find((node: { id: string }) => node.id === "trade");
		expect(trade?.n).toBe(9);
		expect(trade?.last_ns).toBe(1200);
	});

	it("applies a coalesced batch in one pipeline update", () => {
		applyDiagPulses([
			{ id: "trade", n: 3, last_ns: 100 },
			{ id: "ingress", n: 4, ema_ns: 200 },
		]);

		const nodes = atoms.pipeline.get().nodes;
		expect(nodes.find((node: { id: string }) => node.id === "trade")?.n).toBe(3);
		expect(nodes.find((node: { id: string }) => node.id === "ingress")?.ema_ns).toBe(
			200,
		);
		expect(isBursting(diagBursts.state, "trade")).toBe(true);
		expect(isBursting(diagBursts.state, "ingress")).toBe(true);
	});
});
