
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { PositionStopGeometry } from "./position-stop-geometry";

describe("PositionStopGeometry", () => {
	it("renders the floor, peak and stop-level readouts for one lot", () => {
		const markup = renderToStaticMarkup(<PositionStopGeometry symbol="BTC/USD" />);

		expect(markup).toContain('data-f="floor"');
		expect(markup).toContain('data-f="peak"');
		expect(markup).toContain('data-f="profit"');
		expect(markup).toContain('data-f="arm"');
		expect(markup).toContain('data-f="lock"');
		expect(markup).toContain('data-f="trigger"');
		expect(markup).toContain('data-f="stopstatus"');
	});
});
