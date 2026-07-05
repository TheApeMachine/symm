import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import { decisionStore } from "#/collections/decisions";
import { DecisionsSurface } from "./decisions-surface";

describe("DecisionsSurface", () => {
	afterEach(() => {
		decisionStore.actions.reset();
	});

	it("uses the latest decision for the entry line", () => {
		decisionStore.actions.updateFrame({
			uuid: "old",
			role: "decision",
			symbol: "BTC/USD",
			score: 0.2,
			entry_line: 0.111,
			median: 0.222,
			mad: 0.333,
			verdict: "blocked",
		});
		decisionStore.actions.updateFrame({
			uuid: "new",
			role: "decision",
			symbol: "ETH/USD",
			score: 0.4,
			entry_line: 0.777,
			median: 0.888,
			mad: 0.999,
			verdict: "allow",
		});

		const html = renderToString(<DecisionsSurface />);

		expect(html).toContain("0.777");
		expect(html).toContain("median <!-- -->0.888");
		expect(html).toContain("mad <!-- -->0.999");
		expect(html).not.toContain("0.111");
	});
});
