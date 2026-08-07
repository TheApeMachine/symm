import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { PositionStopGeometry } from "./position-stop-geometry";

/*
These are exactly the prices types.Stoploss serialises. The widget is pinned to
that list on purpose: binding a boundary the regulator does not publish reads as
a panel that never received data, which is how the previous geometry went dark.
*/
const regulatorBoundaries = [
	"floor",
	"profit_line",
	"arm_at",
	"lock_floor",
	"peak",
] as const;

describe("PositionStopGeometry", () => {
	it("maps every published regulator boundary onto the position card", () => {
		const markup = renderToStaticMarkup(<PositionStopGeometry />);

		for (const boundary of regulatorBoundaries) {
			expect(markup).toContain(`data-set="holding.stoploss.${boundary}"`);
		}

		expect(markup).toContain("holding.stoploss.locked");
		expect(markup).toContain("holding.stoploss.status");
		expect(markup).toContain("holding.stoploss.mark");
	});

	it("binds no boundary the regulator does not publish", () => {
		const markup = renderToStaticMarkup(<PositionStopGeometry />);

		for (const absent of [
			"hard_floor",
			"break_even_line",
			"profit_failsafe",
			"arm_line",
			"profit_floor",
			"trail_floor",
			"transitions",
			"trigger_reason",
			"basis_confirmed",
			"plan.",
		]) {
			expect(markup).not.toContain(absent);
		}
	});
});
