import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DiagnosticsSurface } from "#/routes/diagnostics";

describe("DiagnosticsSurface", () => {
	it("renders sequencer labels before the first snapshot", () => {
		const markup = renderToStaticMarkup(<DiagnosticsSurface />);

		expect(markup).toContain("System diagnostics");
		expect(markup).toContain("Stage map");
		expect(markup).toContain("Stall log");
		expect(markup).toContain("No lane snapshot yet");
	});
});
