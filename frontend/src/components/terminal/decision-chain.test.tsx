import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DecisionChain } from "./decision-chain";

describe("DecisionChain", () => {
	it("starts compact while retaining the full structural decision trace", () => {
		const markup = renderToStaticMarkup(<DecisionChain index={0} />);

		expect(markup).toContain('aria-expanded="false"');
		expect(markup).toContain("group-data-[selected=true]:grid");
		expect(markup).toContain("1 · structural thesis");
		expect(markup).toContain("2 · evidence graph");
		expect(markup).toContain("3 · graph search");
		expect(markup).toContain("4 · execution + risk");
		expect(markup).toContain('data-paint="trace.mcts.branches.0.action"');
		expect(markup).toContain('data-paint="trace.mcts.branches.1.action"');
		expect(markup).toContain('data-paint="trace.mcts.recommendedAction"');
		expect(markup).toContain('data-paint="trace.graphSupports"');
		expect(markup).toContain('data-paint="trace.graphContradicts"');
		expect(markup).toContain('data-paint="trace.graphConditions"');
		expect(markup).toContain('data-paint="thesisScore"');
		expect(markup).toContain('data-paint="thesisConfidence"');
		expect(markup).toContain('data-paint="entryCost.entryPrice"');
		expect(markup).toContain('data-paint="entryCost.breakEven"');
		expect(markup).not.toContain('data-paint="expectedReturn"');
		expect(markup).not.toContain("edge=");
	});
});
