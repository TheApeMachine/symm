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
	Object.fromEntries(defaultOrigins.map((origin) => [origin, Circular(measurementCapacity)])) as Record<
		string,
		ReturnType<typeof Circular>
	>;

export const measurementsStore = createStore(
	{
		measurements: measurementOrigins(),
		symbols: {} as Record<string, Record<string, unknown>[]>,
	},
	({ setState }) => ({
		updateFrame: (measurement: Record<string, unknown>) =>
			setState((prev) => {
				const origin = String(measurement.origin ?? "").trim();
				const measurements = prev.measurements;

				if (origin !== "") {
					if (measurements[origin] === undefined) {
						measurements[origin] = Circular(measurementCapacity);
					}

					measurements[origin].push(measurement);
				}

				const symbol = String(measurement.symbol ?? measurement.scope ?? "").trim();

				if (symbol === "") {
					return { ...prev, measurements };
				}

				return {
					...prev,
					measurements,
					symbols: {
						...prev.symbols,
						[symbol]: [...(prev.symbols[symbol] ?? []), measurement].slice(-measurementCapacity),
					},
				};
			}),
	}),
);
