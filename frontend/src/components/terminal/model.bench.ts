import { bench, describe } from "vitest";
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

const readings = {
  causal: sampleReading("causal"),
  correlation: sampleReading("correlation"),
  cvd: sampleReading("cvd"),
  depthflow: sampleReading("depthflow"),
  exhaustion: sampleReading("exhaustion"),
  fluid: sampleReading("fluid"),
  hawkes: sampleReading("hawkes"),
  leadlag: sampleReading("leadlag"),
  liquidity: sampleReading("liquidity"),
  manifold: sampleReading("manifold"),
  pumpdump: sampleReading("pumpdump"),
  sentiment: sampleReading("sentiment"),
  toxicity: sampleReading("toxicity"),
};

const decisions = Array.from({ length: 64 }, (_, index) => ({
  symbol: `SYM${index}/USD`,
  source: "pumpdump",
  score: index / 64,
  allow: index % 2 === 0,
  in_play: index % 3 === 0,
  why: "benchmark",
}));

describe("terminal model", () => {
  bench("projects websocket stores into terminal rows", () => {
    terminalKernelsFromReadings(readings);
    terminalDecisionRows(decisions);
  });
});
