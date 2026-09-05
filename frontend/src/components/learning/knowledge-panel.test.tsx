import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { KnowledgePanel, PriorFacts } from "./knowledge-panel";
import type { LearningView, Prior } from "./state";

describe("PriorFacts", () => {
	it("separates variance availability, retained authority, depth and pending count", () => {
		const prior = {
			Defined: true,
			Mean: 0,
			VarianceDefined: false,
			Support: 1,
			EvidenceAuthority: 0.5,
			Authority: 0.25,
			Depth: 3,
			Samples: 1,
			Pending: 2,
		} as Prior;
		const html = renderToStaticMarkup(<PriorFacts prior={prior} />);
		expect(html).toContain("variance unestimable");
		expect(html).toContain("retained evidence 50.0%");
		expect(html).toContain("depth 3");
		expect(html).toContain("pending 2");
	});
});
describe("KnowledgePanel", () => {
	it("reports the scope actually selected without summing alternative evidence", () => {
		const prior = { Defined: false } as Prior;
		const view = {
			candidates: [
				{
					kind: "buy",
					power: 0,
					reduce: false,
					knowledge: {
						scope: "symbol",
						global: prior,
						symbol: prior,
						selected: prior,
					},
				},
			],
			warmup: { resolved: 12, unconditioned: 2, portfolioUnavailable: 12 },
		} as unknown as LearningView;
		const html = renderToStaticMarkup(<KnowledgePanel view={view} />);
		expect(html).toContain("selected symbol");
		expect(html).toContain("12 complete experiences");
		expect(html).toContain(
			"Historical knowledge grants no live entry authority",
		);
	});
});
