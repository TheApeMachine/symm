import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DiagnosticsSurface } from "#/routes/diagnostics";

describe("DiagnosticsSurface", () => {
	it("renders the wiring map labels before any frame arrives", () => {
		const markup = renderToStaticMarkup(<DiagnosticsSurface />);

		expect(markup).toContain("System diagnostics");
		expect(markup).toContain("Pipeline wiring map");
		expect(markup).toContain("Ingress");
		expect(markup).toContain("execution");
		expect(markup).toContain("wire latency");
		// Writer/reader legend appears regardless of the frame contents.
		expect(markup).toContain("writer");
		expect(markup).toContain("reader");
	});

	it("shows the item count stat strip", () => {
		const markup = renderToStaticMarkup(<DiagnosticsSurface />);

		expect(markup).toContain("stages live");
		expect(markup).toContain("items pending");
		expect(markup).toContain("queues under pressure");
	});

	it("waits for the queue pressure board when no queues have been reported", () => {
		const markup = renderToStaticMarkup(<DiagnosticsSurface />);

		expect(markup).toContain(
			"Waiting for the diagnostics WebRTC frame to report queue pressure",
		);
	});
});
