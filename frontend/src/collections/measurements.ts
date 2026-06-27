import { createStore } from "@tanstack/react-store";

export type MeasurementHistorySample = {
	confidence?: number;
	surprise?: number;
	strength?: number;
	timestamp?: string;
	observed_at?: number;
	[key: string]: unknown;
};

/*
measurementsStore indexes raw role=measurement artifacts by origin → scope. The
backend owns measurement history; the frontend stores only the latest raw frame
per origin/scope and renders artifact fields (output.confidence, output.surprise,
output.strength, output.category, timestamp) directly. No accumulation, no
sampling, no derivation here.
*/
export const measurementsStore = createStore(
	{} as Record<string, Record<string, Record<string, unknown>>>,
	({ setState }) => ({
		updateReading: (frame: Record<string, unknown>) =>
			measurementsStore.actions.updateReadings([frame]),
		updateReadings: (frames: Record<string, unknown>[]) => {
			if (frames.length === 0) {
				return;
			}

			setState((prev) => {
				const next = { ...prev };
				const touched = new Set<string>();

				for (const frame of frames) {
					const origin = typeof frame.origin === "string" ? frame.origin : "";
					const scope = typeof frame.scope === "string" ? frame.scope : "";

					if (origin === "" || scope === "") {
						continue;
					}

					const byScope =
						next[origin] === undefined || touched.has(origin)
							? (next[origin] ?? {})
							: { ...next[origin] };
					touched.add(origin);
					byScope[scope] = frame;
					next[origin] = byScope;
				}

				return touched.size === 0 ? prev : next;
			});
		},
		reset: () => {
			setState(() => ({}));
		},
	}),
);
