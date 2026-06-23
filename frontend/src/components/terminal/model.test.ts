import { describe, expect, it } from "vitest";
import type { SignalReading } from "#/collections/signals";
import {
  terminalDecisionRows,
  terminalKernelsFromReadings,
} from "#/components/terminal/model";

const sampleReading = (source: string): SignalReading => ({
  source,
  confidence: 0.42,
  surprise: 1.8,
  surpriseThreshold: 2,
  strength: 0.4,
  elapsed: 60,
  category: "laminar",
  activeReadings: 1,
  readingsCapacity: 8,
  observedAt: Date.now(),
  bestEffort: false,
  gapReason: "",
  samples: 12,
  minSamples: 24,
  calibrating: false,
  calibrated: true,
  updatedAt: Date.now(),
});

describe("terminal model", () => {
  it("projects signal readings into kernel rows without inventing sources", () => {
    const kernels = terminalKernelsFromReadings({
      pumpdump: sampleReading("pumpdump"),
    });
    const pumpKernel = kernels.find((kernel) => kernel.source === "pumpdump");
    const fluidKernel = kernels.find((kernel) => kernel.source === "fluid");

    expect(pumpKernel?.name).toBe("Pump impulse");
    expect(pumpKernel?.status).toBe("healthy");
    expect(pumpKernel?.confidenceText).toBe("0.42");
    expect(pumpKernel?.samplesText).toBe("12/24");
    expect(fluidKernel?.status).toBe("waiting");
  });

  it("does not label raw measurement evidence as waiting", () => {
    const kernels = terminalKernelsFromReadings({
      pumpdump: {
        ...sampleReading("pumpdump"),
        calibrated: false,
      },
    });
    const pumpKernel = kernels.find((kernel) => kernel.source === "pumpdump");

    expect(pumpKernel?.status).toBe("healthy");
    expect(pumpKernel?.confidenceText).toBe("0.42");
  });

  it("sorts backend decision rows and preserves verdict state", () => {
    const rows = terminalDecisionRows([
      {
        symbol: "ETH/USD",
        source: "pumpdump",
        score: 0.18,
        allow: false,
        in_play: false,
        why: "below_edge",
      },
      {
        symbol: "SOL/USD",
        source: "causal",
        score: 0.64,
        allow: true,
        in_play: true,
        why: "matched_branch",
        signals: [{ source: "causal", confidence: 0.64 }],
      },
    ]);

    expect(rows.map((row) => row.symbol)).toEqual(["SOL/USD", "ETH/USD"]);
    expect(rows[0]?.verdict).toBe("in-play");
    expect(rows[0]?.signals).toEqual([{ source: "causal", confidence: 0.64 }]);
    expect(rows[1]?.verdict).toBe("blocked");
    expect(rows[1]?.why).toBe("below edge");
  });
});
