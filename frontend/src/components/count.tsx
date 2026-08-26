import { positionsStore, useSubscribe } from "#/providers/ws-stores";

/*
Count reads the open-lot tally off the positions store — the only place that
knows how many lots are actually held. It subscribes and writes the count into
its own span directly; React never re-renders.
*/
export const Count = () => {
	const root = useSubscribe(positionsStore, (state) => {
		const value = root.current?.querySelector<HTMLElement>("[data-count]");

		if (value === null || value === undefined) {
			return;
		}

		const open = Object.values(state.positions)
			.map((buffer) => buffer.latest())
			.filter((row) => row !== undefined && row.status === "open");

		value.textContent = String(open.length);
	});

	return (
		<span ref={root} className="font-mono text-[12px] text-(--f3)">
			<span data-count className="text-(--f1)">
				—
			</span>{" "}
			open positions
		</span>
	);
};
