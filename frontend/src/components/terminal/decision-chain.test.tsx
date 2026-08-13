import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DecisionChain } from "./decision-chain";

describe("DecisionChain", () => {
	it("starts compact while retaining the full four-stage trace for selection", () => {
		const markup = renderToStaticMarkup(<DecisionChain index={0} />);

		expect(markup).toContain('aria-expanded="false"');
		expect(markup).toContain("group-data-[selected=true]:grid");
		expect(markup).toContain("1 · forecast");
		expect(markup).toContain("2 · evidence graph");
		expect(markup).toContain("3 · graph search");
		expect(markup).toContain("4 · capital + slots");
		expect(markup).toContain('data-paint="trace.mcts.branches.0.action"');
		expect(markup).toContain('data-paint="trace.mcts.branches.1.action"');
		expect(markup).toContain('data-paint="trace.mcts.recommendedAction"');
		expect(markup).toContain('data-paint="trace.graphSupports"');
		expect(markup).toContain('data-paint="trace.graphContradicts"');
		expect(markup).not.toContain('data-paint="alternatives.enter"');
		expect(markup).not.toContain('data-paint="trace.utility.executableFraction"');
		expect(markup).toContain('data-paint="allocation_haircut"');
		expect(markup).toContain('data-paint="allocation_haircut_reason"');
		expect(markup).toContain('data-paint="forecastHorizon"');
		expect(markup).toContain('data-paint="graphScore"');
		expect(markup).toContain('data-paint="utility"');
	});
});
