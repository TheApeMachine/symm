import { createStore } from "@tanstack/react-store";
import { Circular } from "./circular";

export const decisionStore = createStore(
  {
    tick: null as number | null,
    decisions: Circular(50),
    allowed: [] as Record<string, unknown>[],
    denied: [] as Record<string, unknown>[],
  },
  ({ setState }) => ({
    observeTick: (tick: unknown) =>
      setState((prev) => {
        const count = Number(tick);

        if (!Number.isFinite(count) || prev.tick === count) {
          return prev;
        }

        return {
          tick: count,
          decisions: Circular(50),
          allowed: [],
          denied: [],
        };
      }),
    updateFrame: (decision: Record<string, unknown>) =>
      setState((prev) => {
        const tick = Number(decision.tick);
        const next =
          Number.isFinite(tick) && prev.tick !== tick
            ? {
                tick,
                decisions: Circular(50),
                allowed: [] as Record<string, unknown>[],
                denied: [] as Record<string, unknown>[],
              }
            : prev;

        next.decisions.push(decision);

        return {
          ...next,
          allowed:
            decision.verdict === "allow"
              ? [...next.allowed, decision].slice(-50)
              : next.allowed,
          denied:
            decision.verdict === "allow"
              ? next.denied
              : [...next.denied, decision].slice(-50),
        };
      }),
    reset: () =>
      setState(() => ({
        tick: null,
        decisions: Circular(50),
        allowed: [],
        denied: [],
      })),
  }),
);
