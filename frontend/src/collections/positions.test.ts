import { beforeEach, describe, expect, it } from "vitest";
import { balanceStore } from "#/collections/balance";
import {
  applyBalanceFrame,
  resetPositionStateForTest,
} from "#/collections/positions";
import { statusStore } from "#/collections/status";

describe("dashboard positions", () => {
  beforeEach(() => {
    resetPositionStateForTest();
  });

  it("renders open inventory from balance frames", () => {
    applyBalanceFrame({
      type: "balances",
      balanceLabel: "$149.87",
      symbol: "$",
      Currency: "USD",
      Balance: 149.87,
      Inventory: { BTC: 0.001 },
      AvgEntry: { BTC: 50130 },
    });

    expect(statusStore.state.positionViews).toEqual([
      {
        symbol: "BTC/USD",
        qty: 0.001,
        avgEntry: 50130,
        mark: 0,
        exitValue: 0,
        unrealized: 0,
        unrealizedPct: 0,
        priced: false,
        exitFeeRate: 0,
      },
    ]);
    expect(balanceStore.state.openPositions).toBe(1);
    expect(balanceStore.state.pricedPositions).toBe(0);
    expect(balanceStore.state.liquidationBalance).toBe(149.87);
    expect(balanceStore.state.liquidationComplete).toBe(false);
    expect(balanceStore.state.exitBalance).toBe(0);
    expect(balanceStore.state.inProfit).toBe(true);
  });

  it("uses expected exit value after bid-side fee instead of optimistic mark P/L", () => {
    applyBalanceFrame({
      type: "balances",
      balanceLabel: "$149.87",
      symbol: "$",
      Currency: "USD",
      Balance: 149.87,
      Inventory: { BTC: 0.001 },
      AvgEntry: { BTC: 50130 },
      Marks: { "BTC/USD": 50630 },
      ExpectedExit: { BTC: 50.498362 },
      Unrealized: { BTC: 0.368362 },
      ExitFeeRate: { BTC: 0.0026 },
    });

    const position = statusStore.state.positionViews[0];

    if (position === undefined) {
      throw new Error("expected one open position");
    }

    expect(position.priced).toBe(true);
    expect(position.mark).toBe(50630);
    expect(position.exitValue).toBe(50.498362);
    expect(position.exitFeeRate).toBe(0.0026);
    expect(position.unrealized).toBeCloseTo(0.368362);
    expect(position.unrealizedPct).toBeCloseTo(0.7348, 4);
    expect(balanceStore.state.pricedPositions).toBe(1);
    expect(balanceStore.state.liquidationBalance).toBeCloseTo(200.368362);
    expect(balanceStore.state.liquidationComplete).toBe(true);
    expect(balanceStore.state.exitBalance).toBeCloseTo(0.368362);
    expect(balanceStore.state.inProfit).toBe(true);
  });
});
