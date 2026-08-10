import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Gate } from "#/components/ui/gate";

describe("Gate", () => {
	it("binds presence without inventing a readiness value", () => {
		const markup = renderToStaticMarkup(<Gate bind="symbol" presence />);

		expect(markup).toContain('data-set="symbol"');
		expect(markup).toContain('data-set-scale="presence"');
		expect(markup).toContain('data-target="dataset.gate"');
		expect(markup).not.toContain("data-paint=");
	});
});
