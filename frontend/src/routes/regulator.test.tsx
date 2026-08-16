import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { RegulatorSurface } from "./regulator";

describe("RegulatorSurface", () => {
	it("renders predictive controls and mark-level position context before the first frame", () => {
		const markup = renderToStaticMarkup(<RegulatorSurface />);

		expect(markup).toContain("Global Predictive-Coding Regulator");
		expect(markup).toContain("Position Marks");
		expect(markup).toContain("Mark-level regulator context");
		expect(markup).toContain("floor distance");
		expect(markup).toContain("surge");
	});
});
