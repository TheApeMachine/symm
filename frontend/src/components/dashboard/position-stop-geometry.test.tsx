
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { PositionStopGeometry } from "./position-stop-geometry";

const stopMarkers = ["floor", "peak"] as const;

describe("PositionStopGeometry", () => {
	it("maps the active floor-to-peak interval onto the position card", () => {
		const markup = renderToStaticMarkup(<PositionStopGeometry />);

		for (const boundary of stopMarkers) {
			expect(markup).toContain(`data-set="holding.stoploss.${boundary}"`);
		}

		expect(markup).toContain('data-set="holding.entry_price"');
		expect(markup).toContain('data-set="holding.mark"');
		expect(markup).toContain(
			'data-set-domain="holding.stoploss.floor,holding.stoploss.peak"',
		);
	});

	it("keeps profit-lock boundaries outside the active stop domain", () => {
		const markup = renderToStaticMarkup(<PositionStopGeometry />);

		for (const boundary of ["profit_line", "arm_at", "lock_floor"]) {
			expect(markup).not.toContain(`data-set="holding.stoploss.${boundary}"`);
			expect(markup).toContain(`data-paint="holding.stoploss.${boundary}"`);
		}

		expect(markup).toContain("holding.stoploss.locked");
		expect(markup).toContain("holding.stoploss.status");
		expect(markup).toContain("holding.stoploss.surge_armed");
		expect(markup).toContain("holding.stoploss.momentum_floor");
		expect(markup).toContain("holding.stoploss.last_move");
		expect(markup).toContain("holding.stoploss.trigger_reason");
	});
});
