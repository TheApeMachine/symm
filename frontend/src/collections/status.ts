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
			unrealized: number;
			unrealizedPct: number;
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
				unrealized: number;
				unrealizedPct: number;
			}>,
		) =>
			setState((prev) => ({
				...prev,
				positionViews: positionViews,
			})),
	}),
);
