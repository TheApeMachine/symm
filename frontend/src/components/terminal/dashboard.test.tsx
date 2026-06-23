import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DashboardSurface } from "#/components/terminal/dashboard";
import type { TerminalModel } from "#/components/terminal/model";
import { SurfaceBody } from "#/components/terminal/surfaces";

const model: TerminalModel = {
  online: true,
  clockText: "13:30:00",
  wallet: {
    cash: "$0.00",
    available: "$0.00",
    reserved: "0.00 EUR",
    tick: "0",
    openText: "0 open positions",
  },
  engine: {
    phase: "stream",
    sequence: "#0",
    measurements: 10,
    candidates: 0,
    open: 0,
    signalsText: "10/14",
    signalsPercent: 71,
    fluidText: "3",
    fluidPercent: 37,
  },
  health: {
    healthy: 2,
    total: 14,
    averageConfidence: 23,
    firing: 1,
    warming: 0,
    degraded: 0,
    label: "Thin",
  },
  kernels: [],
  decisions: [],
  positions: [],
  totalPnlText: "$0.0000",
  totalPnlPositive: true,
  auditRows: [],
  cognitive: null,
  cognitiveScopes: [],
  playbookBranches: 0,
  walkSymbol: "",
};

describe("DashboardSurface", () => {
  it("renders the mockup pulse strip and owned chart panels", () => {
    const html = renderToStaticMarkup(
      <DashboardSurface
        model={model}
        selectedSource="pumpdump"
        inspectorSource={null}
        onInspect={() => undefined}
        onCloseInspect={() => undefined}
        onOpenInsight={() => undefined}
      />,
    );

    expect(html).toContain("meas 10");
    expect(html).toContain("Fluid density field");
    expect(html).toContain("Predictive coding");
    expect(html).not.toContain("<h1");
  });

  it("renders x-ray and cognitive context strips", () => {
    const xray = renderToStaticMarkup(
      <SurfaceBody
        surface="xray"
        model={model}
        selectedSource="pumpdump"
        inspectorSource={null}
        onSelectKernel={() => undefined}
        onInspectKernel={() => undefined}
        onCloseInspect={() => undefined}
        onOpenInsight={() => undefined}
      />,
    );
    const cortex = renderToStaticMarkup(
      <SurfaceBody
        surface="cortex"
        model={model}
        selectedSource="pumpdump"
        inspectorSource={null}
        onSelectKernel={() => undefined}
        onInspectKernel={() => undefined}
        onCloseInspect={() => undefined}
        onOpenInsight={() => undefined}
      />,
    );

    expect(xray).toContain("Inspect symbol");
    expect(cortex).toContain("Sensory context");
  });
});
