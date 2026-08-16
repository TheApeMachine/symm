import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AllocationMain } from "./allocation-main";

describe("AllocationMain", () => {
	it("reports the structural thesis rather than the retired future-utility score", () => {
		const markup = renderToStaticMarkup(<AllocationMain />);

		expect(markup).toContain('data-paint="thesisScore"');
		expect(markup).toContain('data-paint="allocationClass"');
		expect(markup).not.toContain('data-paint="utility"');
	});
});
