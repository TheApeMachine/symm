import { createStore } from "@tanstack/react-store";

export type Balance = Record<string, unknown> & {
	asset: string;
	balance: number;
	available?: number;
	reserved?: number;
};

export const balancesStore = createStore(
	{
		balances: [] as Balance[],
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (balances: Balance[]) =>
			setState(() => ({
				balances,
				observed: true,
			})),
	}),
);
