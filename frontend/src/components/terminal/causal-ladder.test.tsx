import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CausalLadder } from "./causal-ladder";
import { setDecisionsScopeSymbol } from "./decision-side";

describe("CausalLadder", () => {
	it("shows dimensionally distinct causal estimates without probability bars", () => {
		setDecisionsScopeSymbol("BTC/USD");
		const markup = renderToStaticMarkup(<CausalLadder />);
		setDecisionsScopeSymbol(undefined);

		expect(markup).toContain('data-f="counterfactual"');
		expect(markup).toContain('data-f="noiseScore"');
		expect(markup).toContain("ρ(treatment, target)");
		expect(markup).not.toContain("P(y");
	});
});
