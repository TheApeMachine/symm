import { describe, expect, it } from "vitest";

import { Circular } from "#/collections/circular";
import { balancesStore } from "#/collections/balances";
import { decisionStore } from "#/collections/decisions";
import { executionsStore } from "#/collections/executions";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { positionsStore } from "#/collections/positions";
import { tickStore } from "#/collections/tick";
import { routeMessage } from "./websocket";

describe("WsFeed message routing", () => {
  it("routes typed dashboard messages into stores", () => {
    measurementsStore.setState(() => ({
      measurements: {
        causal: Circular(50),
        correlation: Circular(50),
        cvd: Circular(50),
        depthflow: Circular(50),
        exhaustion: Circular(50),
        fluid: Circular(50),
        hawkes: Circular(50),
        leadlag: Circular(50),
        liquidity: Circular(50),
        manifold: Circular(50),
        pumpdump: Circular(50),
        regime: Circular(50),
        resonance: Circular(50),
        sentiment: Circular(50),
        toxicity: Circular(50),
      },
      symbols: {},
    }));
    balancesStore.actions.reset();
    executionsStore.actions.reset();
    ordersStore.actions.reset();
    positionsStore.actions.reset();
    tickStore.actions.reset();

    routeMessage({
      measurement: { source: "fluid", symbol: "BTC/USD" },
    });
    routeMessage({ tick: { count: 1 } });
    routeMessage({
      decision: {
        id: "decision-1",
        tick: 1,
        symbol: "BTC/USD",
        verdict: "allow",
      },
    });
    routeMessage({
      measurement: { source: "hawkes", symbol: "ETH/USD" },
    });
    routeMessage({
      regime: { confidence: 0.8, strength: 0.3 },
    });
    routeMessage({
      balances: { rows: [{ asset: "USD", balance: 200 }] },
    });
    routeMessage({
      orders: { rows: [{ order_id: "order-1" }] },
    });
    routeMessage({
      executions: { rows: [{ exec_id: "exec-1" }] },
    });
    routeMessage({
      positions: {
        positions: [{ symbol: "BTC/USD", quantity: 0.01 }],
        count: 1,
        quote: "USD",
      },
    });
    routeMessage({ tick: { count: 2 } });

    expect(measurementsStore.state.measurements.fluid.values()).toEqual([
      { source: "fluid", symbol: "BTC/USD" },
    ]);
    expect(measurementsStore.state.measurements.hawkes.values()).toEqual([
      { source: "hawkes", symbol: "ETH/USD" },
    ]);
    expect(measurementsStore.state.measurements.regime.values()).toEqual([
      {
        confidence: 0.8,
        strength: 0.3,
        source: "regime",
        symbol: "regime",
        category: "regime",
        status: "measured",
      },
    ]);
    expect(tickStore.state.frame).toEqual({ count: 2 });
    expect(balancesStore.state.frame).toEqual({
      rows: [{ asset: "USD", balance: 200 }],
    });
    expect(ordersStore.state.frame).toEqual({
      rows: [{ order_id: "order-1" }],
    });
    expect(executionsStore.state.frame).toEqual({ exec_id: "exec-1" });
    expect(executionsStore.state.history).toEqual([{ exec_id: "exec-1" }]);
    expect(decisionStore.state.decisions.values()).toEqual([]);
    expect(positionsStore.state.frame).toEqual({
      positions: [{ symbol: "BTC/USD", quantity: 0.01 }],
      count: 1,
      quote: "USD",
    });
  });
});
