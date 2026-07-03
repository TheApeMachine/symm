import { createStore } from "@tanstack/react-store";
import { Circular } from "./circular";

export const measurementsStore = createStore(
	{
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
			resonance: Circular(50),
			sentiment: Circular(50),
			toxicity: Circular(50),
		} as Record<string, ReturnType<typeof Circular>>,
		symbols: {} as Record<string, Record<string, unknown>[]>,
	},
	({ setState }) => ({
		updateFrame: (measurement: Record<string, unknown>) =>
			setState((prev) => {
				prev.measurements[measurement.origin as string]?.push(measurement);

				const symbol = String(measurement.symbol ?? measurement.scope ?? "");

				if (symbol === "") {
					return { ...prev };
				}

				return {
					...prev,
					symbols: {
						...prev.symbols,
						[symbol]: [...(prev.symbols[symbol] ?? []), measurement].slice(-50),
					},
				};
			}),
	}),
);
