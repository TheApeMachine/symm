import { describe, expect, it } from "vitest";
import { terminalCrossSectionTiles } from "#/components/terminal/cross-section";

describe("terminalCrossSectionTiles", () => {
  it("uses only live symbol rows from backend cross-section frames", () => {
    const tiles = terminalCrossSectionTiles({
      type: "fluid",
      symbols: [
        { symbol: "BTC/EUR", vol: 1000, change_pct: 1.2, re: 4 },
        { symbol: "ETH/EUR", vol: 500, change_pct: -0.4, re: 2 },
      ],
    });

    expect(tiles).toHaveLength(2);
    expect(tiles.map((tile) => tile.label)).toEqual(["BTC", "ETH"]);
    expect(tiles.every((tile) => tile.value > 0)).toBe(true);
  });

  it("does not fabricate tiles when the frame has no symbols", () => {
    expect(terminalCrossSectionTiles({ type: "state" })).toEqual([]);
    expect(terminalCrossSectionTiles(null)).toEqual([]);
  });
});
