import { createStore } from "@tanstack/react-store";

export const statusStore = createStore(
  {
    actions: [] as Array<{
      type: string;
      symbol: string;
      reason?: string;
      verdict: "filled" | "submitted" | "rejected";
      ts: number;
    }>,
    positionViews: [] as Array<{
      symbol: string;
      qty: number;
      avgEntry: number;
      mark: number;
      exitValue: number;
      unrealized: number;
      unrealizedPct: number;
      priced: boolean;
      exitFeeRate: number;
      stopPrice?: number;
      peakPrice?: number;
      offset?: number;
      markSource?: string;
    }>,
  },
  ({ setState }) => ({
    updateActions: (
      actions: Array<{
        type: string;
        symbol: string;
        reason?: string;
        verdict: "filled" | "submitted" | "rejected";
        ts: number;
      }>,
    ) =>
      setState((prev) => ({
        ...prev,
        actions: actions,
      })),
    updatePositionViews: (
      positionViews: Array<{
        symbol: string;
        qty: number;
        avgEntry: number;
        mark: number;
        exitValue: number;
        unrealized: number;
        unrealizedPct: number;
        priced: boolean;
        exitFeeRate: number;
        stopPrice?: number;
        peakPrice?: number;
        offset?: number;
        markSource?: string;
      }>,
    ) =>
      setState((prev) => ({
        ...prev,
        positionViews: positionViews,
      })),
  }),
);
