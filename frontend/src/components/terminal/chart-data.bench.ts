import { bench, describe } from "vitest";
import {
  appendPredictionFrame,
  emptyPredictionSeries,
  terminalFluidMatrix,
} from "#/components/terminal/chart-data";

const symbols = Array.from({ length: 80 }, (_, index) => ({
  symbol: `SYM${index}/USD`,
  change_pct: Math.sin(index / 8) * 3,
  vol: 1 + index,
  re: Math.abs(Math.sin(index / 7)) + 0.1,
  div: Math.cos(index / 9) * 0.1,
  vort: Math.sin(index / 11) * 0.2,
  turb: Math.abs(Math.cos(index / 5)) * 0.3,
}));

describe("terminal chart data", () => {
  bench("projects fluid frames into terminal heat matrices", () => {
    terminalFluidMatrix({ symbols });
  });

  bench("buffers prediction frames", () => {
    let series = emptyPredictionSeries();

    for (let index = 0; index < 96; index += 1) {
      series = appendPredictionFrame(series, {
        kind: index % 2 === 0 ? "actual" : "prediction",
        x: index,
        value: Math.sin(index / 12),
      });
    }
  });
});
