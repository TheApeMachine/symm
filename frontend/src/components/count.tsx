import { useSelector } from "@tanstack/react-store";
import { positionStore } from "#/collections/app";

/*
Count reads the open-lot tally off the positions store.
*/
export const Count = () => {
	const last = useSelector(positionStore, (state) =>
		state.findLast((f) => f.rowsLength() > 0),
	);

	return (
		<span className="font-mono text-[12px] text-(--f3)">
			<span data-count className="text-(--f1)">
				{String(last ? last.rowsLength() : 0)}
			</span>{" "}
			open positions
		</span>
	);
};


