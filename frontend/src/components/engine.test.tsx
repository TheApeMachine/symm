import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Engine } from "#/components/engine";

describe("Engine", () => {
	it("renders the run readout rows bound to their own stores", () => {
		const markup = renderToStaticMarkup(<Engine />);

		expect(markup).toContain('data-e="seq"');
		expect(markup).toContain('data-e="phase"');
		expect(markup).toContain('data-e="cand"');
		expect(markup).toContain('data-e="meas"');
		expect(markup).toContain('data-e="open"');
		expect(markup).toContain("seq");
		expect(markup).toContain("phase");
	});
});
