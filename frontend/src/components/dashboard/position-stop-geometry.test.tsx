import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { PositionStopGeometry } from "./position-stop-geometry";

const regulatorBoundaries = [
	"hard_floor",
	"break_even_line",
	"profit_failsafe",
	"profit_line",
	"arm_line",
	"profit_floor",
	"trail_floor",
	"floor",
	"peak",
] as const;

describe("PositionStopGeometry", () => {
	it("maps the complete stop regulator geometry onto the position card", () => {
		const markup = renderToStaticMarkup(<PositionStopGeometry />);

		for (const boundary of regulatorBoundaries) {
			expect(markup).toContain(`data-set="holding.stoploss.${boundary}"`);
		}

		expect(markup).toContain("holding.stoploss.plan.noise_band");
		expect(markup).toContain("holding.stoploss.plan.confirm_marks");
		expect(markup).toContain("holding.stoploss.basis_confirmed");
		expect(markup).toContain("holding.stoploss.transitions.length");
		expect(markup).toContain("holding.stoploss.trigger_reason");
	});
});
