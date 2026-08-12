import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Engine } from "#/components/engine";

describe("Engine", () => {
	it("binds gates to module activity", () => {
		const markup = renderToStaticMarkup(<Engine />);

		expect(markup).toContain('data-set="correlation"');
		expect(markup).toContain('data-set="category"');
		expect(markup).toContain('data-set="planner"');
		expect(markup).toContain('data-set-scale="activity-color"');
	});
});
