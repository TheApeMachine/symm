import { bench, beforeEach, describe } from "vitest";
import {
  applyBalanceFrame,
  resetPositionStateForTest,
} from "#/collections/positions";

const balanceFrame = {
  type: "balances",
  balanceLabel: "$38.22",
  symbol: "$",
  Currency: "USD",
  Balance: 38.22,
  Inventory: {
    CFG: 189.2148,
    GBP: 29.8238,
    USDT: 40.0296,
    ZEC: 0.0969,
  },
  AvgEntry: {
    CFG: 0.21,
    GBP: 1.34,
    USDT: 1.0,
    ZEC: 414.41,
  },
  Marks: {
    "GBP/USD": 1.34,
    "USDT/USD": 1.0,
    "ZEC/USD": 411.84,
  },
  ExpectedExit: {
    GBP: 39.8623113712,
    USDT: 39.9495408,
    ZEC: 39.748913856,
  },
  Unrealized: {
    GBP: -0.0943806288,
    USDT: -0.0800592,
    ZEC: -0.406114144,
  },
  ExitFeeRate: {
    GBP: 0.002,
    USDT: 0.002,
    ZEC: 0.004,
  },
};

describe("dashboard positions", () => {
  beforeEach(() => {
    resetPositionStateForTest();
  });

  bench("derive liquidation summary", () => {
    applyBalanceFrame(balanceFrame);
  });
});
