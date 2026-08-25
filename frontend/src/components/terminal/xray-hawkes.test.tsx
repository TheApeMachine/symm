import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { XrayHawkesPanel } from "./xray-hawkes";

describe("XrayHawkesPanel", () => {
	it("plots conditional intensity with its fitted decay contract", () => {
		const markup = renderToStaticMarkup(<XrayHawkesPanel />);

		expect(markup).toContain(
			'data-stream-value="metrics.conditional_intensity:buy.raw"',
		);
		expect(markup).toContain(
			'data-stream-baseline="metrics.background_rate:buy.raw"',
		);
		expect(markup).toContain('data-stream-decay="metrics.excitation_decay:buy_from_buy.raw"');
		expect(markup).toContain("data-stream-rug");
	});
});
