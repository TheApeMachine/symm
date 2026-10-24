import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { cognitionStore } from "#/collections/cognition";
import { Decisions } from "./decisions";

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => undefined,
}));

describe("Decisions", () => {
	it("renders one card per real decision, with no pre-allocated slots", () => {
		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).not.toContain("data-decision-id");
		expect(markup).toContain("waiting for backend decision frames");
		expect(markup).not.toMatch(
			/data-decision-card="true"[^>]*data-decision-card="true"/,
		);
	});

	it("renders cognition match frames as decision cards", () => {
		cognitionStore.setState((state) => ({
			...state,
			matches: {
				"BTC/USD": {
					symbol: "BTC/USD",
					tick: 42,
					tokens: ["R01", "R02"],
					stored: true,
					match: "policy/current/BTC/USD/enter",
					depth: 2,
					action: "enter",
				},
			},
		}));

		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).toContain('data-decision-id="BTC/USD-42"');
		expect(markup).toContain("BTC/USD");
		expect(markup).toContain("enter");

		cognitionStore.setState((state) => ({ ...state, matches: {} }));
	});
});
