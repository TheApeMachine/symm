import { bench, beforeEach, describe } from "vitest";
import {
  applyBalanceFrame,
  applyPositionFrame,
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

  bench("apply backend monitor frame", () => {
    applyPositionFrame({
      type: "positions",
      currency: "USD",
      cash: 38.22,
      open_positions: 4,
      priced_positions: 4,
      exit_value: 159.92,
      exit_balance: -1.34,
      liquidation_balance: 198.14,
      liquidation_complete: true,
      in_profit: false,
      positions: [
        {
          symbol: "CFG/USD",
          qty: 189.2148,
          avg_entry: 0.21,
          mark: 0.2084,
          exit_value: 39.4303,
          unrealized: -0.304,
          unrealized_pct: -0.7659,
          priced: true,
          stop_price: 0.205,
          peak_price: 0.212,
          offset: 0.015,
        },
        {
          symbol: "GBP/USD",
          qty: 29.8238,
          avg_entry: 1.34,
          mark: 1.337,
          exit_value: 39.8623,
          unrealized: -0.0944,
          unrealized_pct: -0.2362,
          priced: true,
        },
        {
          symbol: "USDT/USD",
          qty: 40.0296,
          avg_entry: 1,
          mark: 0.998,
          exit_value: 39.9495,
          unrealized: -0.0801,
          unrealized_pct: -0.2,
          priced: true,
        },
        {
          symbol: "ZEC/USD",
          qty: 0.0969,
          avg_entry: 414.41,
          mark: 411.84,
          exit_value: 39.7489,
          unrealized: -0.4061,
          unrealized_pct: -1.0109,
          priced: true,
        },
      ],
    });
  });
});
