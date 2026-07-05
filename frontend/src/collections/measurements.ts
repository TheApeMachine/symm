import { createStore } from "@tanstack/react-store";
import { Circular } from "./circular";

const measurementCapacity = 50;

const defaultOrigins = [
  "causal",
  "correlation",
  "cvd",
  "depthflow",
  "exhaustion",
  "fluid",
  "hawkes",
  "leadlag",
  "liquidity",
  "manifold",
  "pumpdump",
  "regime",
  "resonance",
  "sentiment",
  "toxicity",
];

export type MeasurementsState = {
  measurements: Record<string, ReturnType<typeof Circular>>;
  symbols: Record<string, Record<string, unknown>[]>;
};

export const measurementOrigins = () =>
  Object.fromEntries(
    defaultOrigins.map((origin) => [origin, Circular(measurementCapacity)]),
  ) as Record<string, ReturnType<typeof Circular>>;

export const measurementsStore = createStore(
  {
    measurements: measurementOrigins(),
    symbols: {} as Record<string, Record<string, unknown>[]>,
  },
  ({ setState }) => ({
    updateFrame: (measurement: Record<string, unknown>) =>
      setState((prev) => {
        const source = String(measurement.source ?? "").trim();
        const measurements = prev.measurements;

        if (source !== "") {
          if (measurements[source] === undefined) {
            measurements[source] = Circular(measurementCapacity);
          }

          measurements[source].push(measurement);
        }

        const symbol = String(measurement.symbol ?? "").trim();

        if (symbol === "") {
          return { ...prev, measurements };
        }

        return {
          ...prev,
          measurements,
          symbols: {
            ...prev.symbols,
            [symbol]: [...(prev.symbols[symbol] ?? []), measurement].slice(
              -measurementCapacity,
            ),
          },
        };
      }),
  }),
);
