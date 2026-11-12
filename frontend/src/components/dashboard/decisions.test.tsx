import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import { Decisions } from "./decisions";

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => () => undefined,
}));

describe("Decisions", () => {
	beforeEach(() => {
		bookAtom.set("virtual");
		signals.decisions.setState(() => ({}));
		signals.regions.setState(() => ({}));
	});

	it("renders one card per real decision, with no pre-allocated slots", () => {
		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).not.toContain("data-decision-id");
		expect(markup).toContain("waiting for backend decision frames");
		expect(markup).not.toMatch(
			/data-decision-card="true"[^>]*data-decision-card="true"/,
		);
	});

	it("renders live decision frames including denials for the selected book", () => {
		signals.decisions.setState(() => ({
			"virtual:BTC/USD": {
				symbol: "BTC/USD",
				action: "skip",
				reason: "oracle veto lit — not tradable",
				gate: "oracle",
				tier: "virtual",
				confidence: 0.82,
			},
			"virtual:ETH/USD": {
				symbol: "ETH/USD",
				action: "enter",
				reason: "tree enter · virtual sample",
				path: "R01/R02",
				tier: "virtual",
				confidence: 0.71,
			},
			"paper:ETH/USD": {
				symbol: "ETH/USD",
				action: "enter",
				reason: "paper enter",
				tier: "paper",
				path: "R01/R02",
			},
		}));

		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).toContain("BTC/USD");
		expect(markup).toContain("skip");
		expect(markup).toContain("oracle:");
		expect(markup).toContain("oracle veto lit");
		expect(markup).toContain("ETH/USD");
		expect(markup).toContain("enter");
		expect(markup).toContain("virtual sample");
		expect(markup).not.toContain("paper enter");
	});

	it("hides other ledgers when the book tab changes", () => {
		signals.decisions.setState(() => ({
			"virtual:ETH/USD": {
				symbol: "ETH/USD",
				action: "enter",
				reason: "virtual sample",
				tier: "virtual",
			},
			"paper:ETH/USD": {
				symbol: "ETH/USD",
				action: "blocked",
				reason: "paper edge gate",
				tier: "paper",
				gate: "tier",
			},
		}));

		bookAtom.set("paper");
		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).toContain("ETH/USD");
		expect(markup).toContain("blocked");
		expect(markup).toContain("paper edge gate");
		expect(markup).not.toContain("virtual sample");
	});

	it("shows pre-tier gate skips on the selected real book", () => {
		bookAtom.set("real");
		signals.decisions.setState(() => ({
			"BTC/USD": {
				symbol: "BTC/USD",
				action: "skip",
				reason: "oracle veto lit — not tradable",
				gate: "oracle",
			},
		}));

		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).toContain("BTC/USD");
		expect(markup).toContain("skip");
		expect(markup).toContain("oracle veto lit");
	});

	it("paints oracle veto/approve lamps from region brightness", () => {
		signals.decisions.setState(() => ({
			"virtual:AAVE/USD": {
				symbol: "AAVE/USD",
				action: "blocked",
				reason: "resonance: no resonance reading",
				path: "R12/R00",
				gate: "resonance",
				tier: "virtual",
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
	});

	it("paints lamps from sparse grid frames (one side per oracle pair)", () => {
		signals.decisions.setState(() => ({
			"virtual:SOL/USD": {
				symbol: "SOL/USD",
				action: "skip",
				reason: "oracle veto lit — not tradable",
				path: "R06.R00/R07.R01",
				gate: "oracle",
				tier: "virtual",
			},
		}));

		// Grid Observe emits only the winning side of each pair.
		signals.regions.setState(() => ({
			"SOL/USD": [
				{ symbol: "SOL/USD", key: 6, brightness: 0.4, deformation: 1, counts: 1 },
				{ symbol: "SOL/USD", key: 7, brightness: 0.2, deformation: 1, counts: 1 },
				{ symbol: "SOL/USD", key: 8, brightness: 0.1, deformation: 1, counts: 1 },
				{ symbol: "SOL/USD", key: 9, brightness: 1, deformation: 0, counts: 0 },
				{ symbol: "SOL/USD", key: 12, brightness: 1, deformation: 0, counts: 0 },
			],
		}));

		const markup = renderToStaticMarkup(<Decisions />);

		expect(markup).toContain("oracle veto lit");
		expect(markup).toContain('data-oracle-gate="vol"');
		expect(markup).toContain('data-oracle-side="veto"');
		expect(markup).toContain('data-oracle-gate="day"');
		expect(markup).toMatch(
			/data-oracle-gate="day"[^>]*data-oracle-side="approve"|data-oracle-side="approve"[^>]*data-oracle-gate="day"/,
		);
	});
});
