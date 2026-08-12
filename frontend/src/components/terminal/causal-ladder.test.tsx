import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CausalLadder } from "./causal-ladder";
import { setDecisionsScopeSymbol } from "./decision-side";

describe("CausalLadder", () => {
	it("binds the counterfactual rung to the counterfactual estimate", () => {
		setDecisionsScopeSymbol("BTC/USD");
		const markup = renderToStaticMarkup(<CausalLadder />);
		setDecisionsScopeSymbol(undefined);

		expect(markup).toContain('data-paint="counterfactual"');
		expect(markup).toContain('data-set="counterfactual"');
		expect(markup).not.toContain('data-paint="confidence"');
	});
});
