import { createStore } from "@tanstack/react-store";

export type Position = {
	symbol: string;
	qty: number;
	entry_price: number;
	mark: number;
	pnl: number;
	return_pct: number;
};

export const positionsStore = createStore(
	{
		positions: [] as Position[],
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (positions: Position[]) =>
			setState(() => ({
				positions,
				observed: true,
			})),
	}),
);
