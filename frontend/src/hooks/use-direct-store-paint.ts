import { type DependencyList, useLayoutEffect } from "react";

type StoreSubscription = {
	subscribe: (listener: () => void) => {
		unsubscribe: () => void;
	};
};

/*
useDirectStorePaint schedules imperative DOM writes on requestAnimationFrame so
high-frequency store updates bypass React reconciliation while still batching layout.
*/
export const useDirectStorePaint = (
	paint: () => void,
	stores: StoreSubscription[],
	deps: DependencyList,
) => {
	useLayoutEffect(() => {
		let frame: number | null = null;

		const schedule = () => {
			if (frame !== null) {
				return;
			}

			frame = requestAnimationFrame(() => {
				frame = null;
				paint();
			});
		};

		paint();

		const subscriptions = stores.map((store) => store.subscribe(schedule));

		return () => {
			for (const subscription of subscriptions) {
				subscription.unsubscribe();
			}

			if (frame !== null) {
				cancelAnimationFrame(frame);
			}
		};
	}, [...deps, paint, stores.map]);
};
