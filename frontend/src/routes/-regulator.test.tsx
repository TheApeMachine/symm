import { renderToReadableStream } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Route } from "./regulator";

const RegulatorSurface = Route.options.component!;

const render = async () => {
  const stream = await renderToReadableStream(<RegulatorSurface />);
  return new Response(stream).text();
};

describe("RegulatorSurface", () => {
  it("renders predictive controls and mark-level position context before the first frame", async () => {
    const markup = await render();

    expect(markup).toContain("Global Predictive-Coding Regulator");
    expect(markup).toContain("Position Marks");
    expect(markup).toContain("Mark-level regulator context");
    expect(markup).toContain("floor distance");
    expect(markup).toContain("surge");
  });
});
