import { createStore } from "@tanstack/react-store";
import type { Measurement } from "#/types/measurement";
import { Circular } from "./circular";

const measurementCapacity = 50;

export type MeasurementsState = {
	measurements: Record<string, ReturnType<typeof Circular>>;
	symbols: Record<string, Measurement[]>;
	sources: Set<string>;
	tick: number;
};

/*
measurementsStore is the single source of truth for all signal data.
Every Measurement from the backend WebSocket is routed here, indexed
by source and by symbol.
*/
export const measurementsStore = createStore(
	{
		measurements: {} as Record<string, ReturnType<typeof Circular>>,
		symbols: {} as Record<string, Measurement[]>,
		sources: new Set<string>(),
		tick: 0,
	},
	({ setState }) => ({
		ingestBatch: (batch: Measurement[]) =>
			setState((prev) => {
				const measurements = { ...prev.measurements };
				const symbols = { ...prev.symbols };
				const sources = new Set(prev.sources);

				for (const measurement of batch) {
					const source = measurement.source.trim();
					const symbol = measurement.symbol.trim();

					if (source !== "") {
						sources.add(source);

						if (measurements[source] === undefined) {
							measurements[source] = Circular(measurementCapacity);
						}

						measurements[source].push(measurement);
					}

					if (symbol !== "") {
						symbols[symbol] = [
							...(symbols[symbol] ?? []),
							measurement,
						].slice(-measurementCapacity);
					}
				}

				return {
					measurements,
					symbols,
					sources,
					tick: prev.tick + 1,
				};
			}),
		updateFrame: (measurement: Record<string, unknown>) =>
			setState((prev) => {
				const source = String(measurement.source ?? "").trim();
				const measurements = { ...prev.measurements };
				const sources = new Set(prev.sources);

				if (source !== "") {
					sources.add(source);

					if (measurements[source] === undefined) {
						measurements[source] = Circular(measurementCapacity);
					}

					measurements[source].push(measurement);
				}

				const symbol = String(measurement.symbol ?? "").trim();

				if (symbol === "") {
					return { ...prev, measurements, sources };
				}

				return {
					...prev,
					measurements,
					sources,
					symbols: {
						...prev.symbols,
						[symbol]: [
							...(prev.symbols[symbol] ?? []),
							measurement as unknown as Measurement,
						].slice(-measurementCapacity),
					},
				};
			}),
	}),
);
