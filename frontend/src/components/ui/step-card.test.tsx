import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { StepCard } from "./step-card";

describe("StepCard component", () => {
	it("renders index step circle, title, value, and explanation", () => {
		const markup = renderToStaticMarkup(
			<StepCard
				step={1}
				title="Opportunity appeared"
				value="lift · expansion"
				explanation="The opportunity tracker recognized this market shape."
			/>,
		);

		expect(markup).toContain("Opportunity appeared");
		expect(markup).toContain("lift · expansion");
		expect(markup).toContain(
			"The opportunity tracker recognized this market shape.",
		);
		expect(markup).toContain(">1<");
	});

	it("supports status colors", () => {
		const markup = renderToStaticMarkup(
			<StepCard
				step="✓"
				title="Passed Gate"
				value="0.018 expected"
				status="success"
			/>,
		);

		expect(markup).toContain("[--step-tone:var(--success)]");
	});
});
