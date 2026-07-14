import { describe, expect, it } from "vitest";
import { categoriesStore } from "#/collections/categories";
import { measurementsStore } from "#/collections/measurements";
import { categoriesForSymbol } from "#/components/terminal/thesis-snapshot";

describe("categoriesForSymbol", () => {
	it("reads legacy measurement categories when thesis categories are absent", () => {
		categoriesStore.actions.reset();
		measurementsStore.actions.updateFrame([
			{
				source: "fluid",
				metric: "strength",
				symbol: "BTC/USD",
				at: "2026-07-14T12:00:00Z",
				raw: 0.8,
				normalized: 0.8,
				uncertainty: null,
				validity: { state: "valid", readiness: "ready" },
				scale: {
					kind: "instant",
					from: "2026-07-14T12:00:00Z",
					through: "2026-07-14T12:00:00Z",
				},
				categories: [
					{
						type: "laminar",
						confidence: 0.82,
						surprisal: 0.1,
						strength: 0.7,
					},
				],
			},
		]);

		expect(categoriesForSymbol("BTC/USD")).toEqual([
			{
				symbol: "BTC/USD",
				type: "laminar",
				confidence: 0.82,
				surprisal: 0.1,
				strength: 0.7,
				maturity: 0,
			},
		]);
	});
});
