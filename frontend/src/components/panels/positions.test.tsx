import { describe, expect, it } from "vitest";
import {
  positionMoney,
  positionPercent,
  positionPrice,
  positionQuantity,
  signedPositionMoney,
  unsignedPositionPercent,
} from "#/components/panels/positions";

describe("position dropdown precision", () => {
  it("keeps low-priced assets visible below one cent", () => {
    expect(positionPrice(0.00383512, "$")).toBe("$0.00383512");
    expect(positionPrice(0.31987654, "$")).toBe("$0.31987654");
  });

  it("keeps higher-priced marks precise without overextending the row", () => {
    expect(positionPrice(1.23456789, "$")).toBe("$1.234568");
    expect(positionPrice(43.052341, "$")).toBe("$43.0523");
    expect(positionPrice(1673.204321, "$")).toBe("$1673.2043");
  });

  it("shows sub-cent position deltas and quantities", () => {
    expect(positionQuantity(0.024)).toBe("0.02400000");
    expect(positionMoney(-0.32004321, "$")).toBe("-$0.3200");
    expect(signedPositionMoney(0.0054321, "$")).toBe("+$0.0054");
  });

  it("shows percent precision for P&L and unsigned fees", () => {
    expect(positionPercent(-0.804321)).toBe("-0.8043%");
    expect(positionPercent(1.312345)).toBe("+1.3123%");
    expect(unsignedPositionPercent(0.4)).toBe("0.4000%");
  });
});
