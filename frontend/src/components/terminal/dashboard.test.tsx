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

  it("renders the decision tree candidate surface from backend rows", () => {
    const decisionModel: TerminalModel = {
      ...model,
      kernels: [
        {
          source: "pumpdump",
          name: "Pump impulse",
          category: "pumpdump",
          status: "healthy",
          statusLabel: "ok",
          strengthText: "0.8910",
          confidencePercent: 37,
          surprisePercent: 75,
          healthPercent: 100,
          confidenceText: "0.37",
          surpriseText: "3.00",
          samplesText: "3",
          activeText: "1/8",
          observedText: "observed",
          faultText: "",
        },
        {
          source: "causal",
          name: "Causal ladder",
          category: "causal",
          status: "healthy",
          statusLabel: "ok",
          strengthText: "0.4043",
          confidencePercent: 25,
          surprisePercent: 66,
          healthPercent: 100,
          confidenceText: "0.25",
          surpriseText: "4.00",
          samplesText: "4",
          activeText: "1/8",
          observedText: "observed",
          faultText: "",
        },
        {
          source: "correlation",
          name: "Correlation field",
          category: "correlation",
          status: "healthy",
          statusLabel: "ok",
          strengthText: "0.1200",
          confidencePercent: 40,
          surprisePercent: 50,
          healthPercent: 100,
          confidenceText: "0.40",
          surpriseText: "3.00",
          samplesText: "3",
          activeText: "1/8",
          observedText: "observed",
          faultText: "",
        },
      ],
      decisions: [
        {
          key: "NEAR/EUR:pumpdump",
          symbol: "NEAR/EUR",
          source: "pumpdump",
          scoreText: "0.589",
          scoreValue: 0.589,
          verdict: "blocked",
          why: "below line",
          signals: [
            { source: "pumpdump", confidence: 0.589 },
            { source: "causal", confidence: 0.237 },
          ],
        },
      ],
      cognitive: {
        scope: "NEAR/EUR",
        sequence: "Z8RW-77JS-HM3K-245Y-KFY4",
        regimePrefix: "breakout",
        regimeCohort: 9,
        ambiguous: false,
        sideline: false,
        entropyBits: 2.02,
        entropyThreshold: 3.6,
        classConfidence: 0.2,
        contrastEvidence: 0,
        lookaheadScore: 0.763,
        lookaheadPaths: 17,
        winnerClass: "breakout",
        prewarmPaths: null,
        prewarmScore: null,
        updatedAt: 0,
      },
    };
    const html = renderToStaticMarkup(
      <SurfaceBody
        surface="decisions"
        model={decisionModel}
        selectedSource="pumpdump"
        inspectorSource={null}
        onSelectKernel={() => undefined}
        onInspectKernel={() => undefined}
        onCloseInspect={() => undefined}
        onOpenInsight={() => undefined}
      />,
    );

    expect(html).toContain("Candidate evaluation");
    expect(html).toContain("universe");
    expect(html).toContain("NEAR/EUR");
    expect(html).toContain("Score attribution");
    expect(html).toContain("backend counterfactual probes unavailable");
    expect(html).toContain("Causal ladder");
    expect(html).toContain("Cognitive beam");
    expect(html).not.toContain("Open positions");
  });
});
