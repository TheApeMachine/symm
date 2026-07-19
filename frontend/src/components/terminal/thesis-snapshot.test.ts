import { describe, expect, it } from "vitest";
import { categoriesForSymbol } from "#/components/terminal/thesis-snapshot";

describe("categoriesForSymbol", () => {
	it("reads legacy measurement categories when thesis categories are absent", () => {
		expect(
			categoriesForSymbol(
				"BTC/USD",
				[],
				[
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
				],
			),
		).toEqual([
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
