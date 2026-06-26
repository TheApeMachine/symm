import { createStore } from "@tanstack/react-store";

export const measurementsStore = createStore(
	{} as Record<string, Record<string, Record<string, unknown>>>,
	({ setState }) => ({
		updateReading: (frame: Record<string, unknown>) => {
			const origin = typeof frame.origin === "string" ? frame.origin : "";
			const scope = typeof frame.scope === "string" ? frame.scope : "";

			if (origin === "" || scope === "") {
				return;
			}

			setState((prev) => ({
				...prev,
				[origin]: {
					...(prev[origin] ?? {}),
					[scope]: frame,
				},
			}));
		},
		reset: () => {
			setState(() => ({}));
		},
	}),
);
