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
			'data-stream-baseline="metrics.baseline_intensity:buy.raw"',
		);
		expect(markup).toContain('data-stream-decay="metrics.decay_rate.raw"');
		expect(markup).toContain("data-stream-rug");
	});
});
