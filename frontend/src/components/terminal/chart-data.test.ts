import { describe, expect, it } from "vitest";
import {
  appendPredictionFrame,
  emptyPredictionSeries,
  resetTerminalFluidMatrix,
  terminalFluidMatrix,
  terminalManifoldMatrix,
  terminalResonanceFrame,
} from "#/components/terminal/chart-data";

const fluidRow = (symbol: string, changePct: number, vol: number) => ({
  symbol,
  change_pct: changePct,
  vol,
  re: Math.abs(changePct) + 0.1,
  div: changePct / 10,
  vort: vol / 100,
  turb: Math.abs(changePct) / 5,
});

describe("terminal chart data", () => {
  it("keeps prediction points sorted by backend x value", () => {
    const series = emptyPredictionSeries();
    const withSecond = appendPredictionFrame(series, {
      kind: "actual",
      x: 20,
      value: 0.4,
    });
    const withFirst = appendPredictionFrame(withSecond, {
      kind: "actual",
      x: 10,
      value: 0.2,
    });

    expect(withFirst.actual.map((point) => point.x)).toEqual([10, 20]);
    expect(withFirst.actual.map((point) => point.value)).toEqual([0.2, 0.4]);
  });

  it("projects fluid symbol frames into the mockup heat field matrix", () => {
    resetTerminalFluidMatrix();

    const matrix = terminalFluidMatrix({
      symbols: [
        fluidRow("ALGO/USD", 0.4, 12),
        fluidRow("DOGE/USD", -1.2, 55),
        fluidRow("SOL/USD", 2.1, 90),
      ],
    });

    expect(matrix).toHaveLength(38);
    expect(matrix[0]).toHaveLength(64);
    expect(matrix.flat().some((value) => value > 0)).toBe(true);
  });

  it("normalizes manifold rho frames locally for terminal canvas charts", () => {
    const matrix = terminalManifoldMatrix({
      rho: [
        [1, 2],
        [3, 5],
      ],
    });

    expect(matrix).toEqual([
      [0, 0.25],
      [0.5, 1],
    ]);
  });

  it("normalizes resonance universe frames to the focus x-ray", () => {
    const frame = terminalResonanceFrame({
      type: "resonance_universe",
      ts: "2026-06-23T07:00:00Z",
      arch: [4, 3],
      symbol_count: 1,
      focus_symbol: "SOL/USD",
      symbols: [
        {
          symbol: "SOL/USD",
          surprise: 0.2,
          energy: 0.3,
          confidence: 0.4,
          strength: 0.5,
          category: "laminar_resonance",
          latent: [0.1, 0.2, 0.3],
        },
      ],
      focus: {
        symbol: "SOL/USD",
        category: "laminar_resonance",
        surprise: 0.2,
        energy: 0.3,
        confidence: 0.4,
        layers: [
          { state: [0.1, 0.2], prediction: [0.2, 0.3], error_norm: 0.1 },
        ],
      },
    });

    expect(frame?.symbol).toBe("SOL/USD");
    expect(frame?.layers).toHaveLength(1);
  });
});
