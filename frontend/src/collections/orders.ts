import { createStore } from "@tanstack/react-store";

export type Order = {
	id: string;
	pair: string;
	price: number;
	reserved_amount: number;
	reserved_asset: string;
	side: string;
	type: string;
	volume: number;
	created_at: string;
};

export const ordersStore = createStore(
	{
		orders: [] as Order[],
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (orders: Order[]) =>
			setState(() => ({
				orders,
				observed: true,
			})),
	}),
);
