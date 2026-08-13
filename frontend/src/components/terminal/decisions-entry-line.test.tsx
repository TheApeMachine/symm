import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { setDecisionsScopeSymbol } from "./decision-side";
import { LiveDecisionsEntryLine } from "./decisions-entry-line";

describe("LiveDecisionsEntryLine", () => {
	it("labels classifier diagnostics without claiming they are planner gates", () => {
		setDecisionsScopeSymbol("BTC/USD");
		const markup = renderToStaticMarkup(<LiveDecisionsEntryLine />);
		setDecisionsScopeSymbol(undefined);

		expect(markup).toContain("runner-up evidence share");
		expect(markup).toContain("strongest standardized channel");
		expect(markup).toContain("winning evidence share");
		expect(markup).not.toContain("entry gate");
	});
});
