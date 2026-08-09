import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DecisionChain } from "./decision-chain";

describe("DecisionChain", () => {
	it("starts compact while retaining the full four-stage trace for selection", () => {
		const markup = renderToStaticMarkup(<DecisionChain index={0} />);

		expect(markup).toContain('aria-expanded="false"');
		expect(markup).toContain("group-data-[selected=true]:grid");
		expect(markup).toContain("1 · forecast");
		expect(markup).toContain("2 · evidence");
		expect(markup).toContain("3 · causal search");
		expect(markup).toContain("4 · capital + slots");
		expect(markup).toContain('data-paint="trace.mcts.precision"');
		expect(markup).toContain('data-paint="trace.mcts.standingAside"');
		expect(markup).toContain('data-paint="trace.mcts.enter"');
		expect(markup).toContain('data-paint="allocation_haircut"');
		expect(markup).toContain('data-paint="allocation_haircut_reason"');
	});
});
