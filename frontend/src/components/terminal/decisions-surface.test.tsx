import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, beforeEach, describe, expect, it } from "vitest";
import { decisionStore } from "#/collections/app";
import { DecisionsSurface } from "#/components/terminal/decisions-surface";
import { DecisionT } from "#/providers/telemetry/telemetry/decision";

const decisionFor = (symbol: string): DecisionT =>
	new DecisionT(`d-${symbol}`, "nothing", symbol);

describe("DecisionsSurface", () => {
	beforeEach(() => {
		decisionStore.actions.reset();
	});

	afterAll(() => {
		decisionStore.actions.reset();
	});

	it("waits explicitly when no frames have arrived", () => {
		expect(renderToStaticMarkup(<DecisionsSurface />)).toContain(
			"waiting for backend decision frames",
		);
	});

	it("keeps a row per symbol instead of mirroring the newest frame", () => {
		// Consecutive frames naming different symbols previously replaced the
		// single visible row each tick, so a decision appeared and was
		// immediately overwritten.
		decisionStore.actions.add(decisionFor("SKR/USD"));
		decisionStore.actions.add(decisionFor("BTC/USD"));

		const markup = renderToStaticMarkup(<DecisionsSurface />);

		expect(markup).toContain("SKR/USD");
		expect(markup).toContain("BTC/USD");
	});

	it("keeps one row per symbol when a symbol is decided repeatedly", () => {
		decisionStore.actions.add(decisionFor("SKR/USD"));
		decisionStore.actions.add(decisionFor("SKR/USD"));
		decisionStore.actions.add(decisionFor("SKR/USD"));

		const markup = renderToStaticMarkup(<DecisionsSurface />);
		const occurrences = markup.split("SKR/USD").length - 1;

		// The symbol paints in its row once; repeated decisions update that
		// row rather than stacking duplicates.
		expect(occurrences).toBeLessThanOrEqual(2);
	});
});
