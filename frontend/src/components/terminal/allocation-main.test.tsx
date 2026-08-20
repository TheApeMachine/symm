import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AllocationMain } from "./allocation-main";

describe("AllocationMain", () => {
	it("reports the structural thesis rather than the retired future-utility score", () => {
		const markup = renderToStaticMarkup(<AllocationMain />);

		expect(markup).toContain("ranked by structural thesis");
		expect(markup).toContain("structural thesis");
		expect(markup).not.toContain("future utility");
	});
});
