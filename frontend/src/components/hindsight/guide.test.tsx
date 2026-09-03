import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Guide } from "./guide";

describe("Guide", () => {
	const markup = renderToStaticMarkup(<Guide onClose={() => {}} />);

	it("names every band of the surface in reading order", () => {
		for (const band of [
			"Run bar",
			"Targets",
			"The chart",
			"Episodes",
			"Signal measurements",
		]) {
			expect(markup).toContain(band);
		}
	});

	it("offers investigations rather than recipes", () => {
		expect(markup).toContain("What are you trying to find out?".toLowerCase());
		expect(markup).toContain("what SYMM knew during it");
	});

	it("never claims a trade was available or missed", () => {
		expect(markup).not.toMatch(
			/missed profit|should have (entered|bought|sold)|missed opportunity/i,
		);
	});

	it("explains that colour describes knowability, not desirability", () => {
		expect(markup).toContain("how well the system could know a number");
	});
});
