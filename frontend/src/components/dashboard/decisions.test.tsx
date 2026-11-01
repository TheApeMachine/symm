import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { signals } from "#/collections/app";
import { Decisions } from "./decisions";

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => undefined,
}));

describe("Decisions", () => {
	it("renders one card per real decision, with no pre-allocated slots", () => {
		signals.decisions.setState(() => ({}));
		signals.regions.setState(() => ({}));

		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).not.toContain("data-decision-id");
		expect(markup).toContain("waiting for backend decision frames");
		expect(markup).not.toMatch(
			/data-decision-card="true"[^>]*data-decision-card="true"/,
		);
	});

	it("renders live decision frames including denials", () => {
		signals.regions.setState(() => ({}));
		signals.decisions.setState(() => ({
			"BTC/USD": {
				symbol: "BTC/USD",
				action: "blocked",
				reason: "blown-out spread: bid-ask spread exceeds 0.5%",
				gate: "physical",
				confidence: 0.82,
			},
			"ETH/USD": {
				symbol: "ETH/USD",
				action: "enter",
				reason: "tree enter · virtual sample",
				path: "R01/R02",
				virtual: true,
				confidence: 0.71,
			},
		}));

		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).toContain("BTC/USD");
		expect(markup).toContain("blocked");
		expect(markup).toContain("physical:");
		expect(markup).toContain("blown-out spread");
		expect(markup).toContain("ETH/USD");
		expect(markup).toContain("enter");

		signals.decisions.setState(() => ({}));
	});

	it("paints oracle veto/approve lamps from region brightness", () => {
		signals.decisions.setState(() => ({
			"AAVE/USD": {
				symbol: "AAVE/USD",
				action: "blocked",
				reason: "resonance: no resonance reading",
				path: "R12/R00",
				gate: "resonance",
			},
		}));

		signals.regions.setState(() => ({
			"AAVE/USD": [
				{ symbol: "AAVE/USD", key: 9, brightness: 0.8, deformation: 1, counts: 1 },
				{ symbol: "AAVE/USD", key: 10, brightness: 0, deformation: 0, counts: 0 },
				{ symbol: "AAVE/USD", key: 12, brightness: 0.6, deformation: 1, counts: 1 },
				{ symbol: "AAVE/USD", key: 11, brightness: 0, deformation: 0, counts: 0 },
			],
		}));

		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).toContain('data-oracle-gate="vol"');
		expect(markup).toContain('data-oracle-side="veto"');
		expect(markup).toContain('data-oracle-gate="day"');
		expect(markup).toMatch(
			/data-oracle-gate="day"[^>]*data-oracle-side="approve"|data-oracle-side="approve"[^>]*data-oracle-gate="day"/,
		);

		signals.decisions.setState(() => ({}));
		signals.regions.setState(() => ({}));
	});
});
