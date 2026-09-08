import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CapitalPanel } from "./capital-panel";
import { learningFixture } from "./fixture";
import { projectLearning } from "./state";

describe("CapitalPanel", () => {
	it("uses the consolidated agent's account and preserves negative profit", () => {
		const view = projectLearning(learningFixture(), "");
		const html = renderToStaticMarkup(<CapitalPanel view={view} />);
		expect(html).toContain("Cash 190");
		expect(html).toContain("Equity 198");
		expect(html).toContain("P&amp;L -2");
		expect(html).toContain("-100.0 bp");
	});
	it("keeps a missing account unmeasured", () => {
		const html = renderToStaticMarkup(<CapitalPanel view={null} />);
		expect(html).toContain("Cash unmeasured");
		expect(html).not.toContain("Cash 0");
	});
});
