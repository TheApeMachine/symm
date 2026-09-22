import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { OutcomeDistribution } from "./outcome-distribution";
describe("OutcomeDistribution", () => {
	it("uses supplied bins with their actual counts, including zero", () => {
		const markup = renderToStaticMarkup(
			<OutcomeDistribution
				bins={[
					{ id: "negative", lower: -2, upper: 0, count: 0 },
					{ id: "positive", lower: 0, upper: 5, count: 10 },
				]}
				unit="bp"
			/>,
		);
		expect(markup).toContain("-2 to 0 bp: 0");
		expect(markup).toContain('height="0"');
		expect(markup).toContain("0 to 5 bp: 10");
		expect(renderToStaticMarkup(<OutcomeDistribution />)).toContain(
			"No outcome distribution",
		);
	});
	it("rejects reversed intervals and negative counts", () => {
		expect(() =>
			renderToStaticMarkup(
				<OutcomeDistribution
					bins={[{ id: "invalid", lower: 3, upper: 1, count: 2 }]}
				/>,
			),
		).toThrow("Invalid outcome bin invalid");
		expect(() =>
			renderToStaticMarkup(
				<OutcomeDistribution
					bins={[{ id: "negative", lower: 0, upper: 1, count: -1 }]}
				/>,
			),
		).toThrow("Invalid outcome bin negative");
	});
});
