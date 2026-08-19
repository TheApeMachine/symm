import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DiagnosticsSurface } from "#/routes/diagnostics";

describe("DiagnosticsSurface", () => {
	it("renders the wiring map labels before any frame arrives", () => {
		const markup = renderToStaticMarkup(<DiagnosticsSurface />);

		expect(markup).toContain("System diagnostics");
		expect(markup).toContain("Wiring map");
		expect(markup).toContain("Ingress");
		expect(markup).toContain("execution");
		expect(markup).toContain("wire latency");
	});
});
