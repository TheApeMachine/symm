import { renderToReadableStream } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Route } from "#/routes/diagnostics";

const DiagnosticsSurface = Route.options.component!;

const render = async () => {
  const stream = await renderToReadableStream(<DiagnosticsSurface />);
  return new Response(stream).text();
};

describe("DiagnosticsSurface", () => {
  it("renders the wiring map labels before any frame arrives", async () => {
    const markup = await render();

    expect(markup).toContain("System diagnostics");
    expect(markup).toContain("Pipeline wiring map");
    expect(markup).toContain("Ingress");
    expect(markup).toContain("execution");
    expect(markup).toContain("wire latency");
    // Writer/reader legend appears regardless of the frame contents.
    expect(markup).toContain("writer");
    expect(markup).toContain("reader");
  });

  it("shows the item count stat strip", async () => {
    const markup = await render();

    expect(markup).toContain("stages live");
    expect(markup).toContain("items pending");
    expect(markup).toContain("queues under pressure");
  });

  it("waits for the queue pressure board when no queues have been reported", async () => {
    const markup = await render();

    expect(markup).toContain(
      "Waiting for the diagnostics WebRTC frame to report queue pressure",
    );
  });
});
