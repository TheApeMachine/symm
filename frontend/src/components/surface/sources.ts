import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { selectDecisions } from "#/components/dashboard/decisions";
import {
	positionsEqual,
	selectPositions,
} from "#/components/dashboard/positions";

/*
The live data a graph-authored surface can ask for, by name.

A component in the library reaches for nothing — that is what makes it portable.
A graph that draws one therefore has to be handed what it draws, and this is the
only place that knows how to get it. A node says `positions`; this says what
`positions` means.

Adding a source is adding an entry here, which is the one thing that cannot come
from the graph: the graph can name a stream, but something has to know which
store answers to that name.
*/
export const useLiveSources = (): Record<string, unknown> => {
	const positions = useSelector(signals.position, selectPositions, {
		compare: positionsEqual,
	});
	const decisions = useSelector(signals.strategy, selectDecisions);

	return {
		positions,
		decisions,
		/*
			What a click on a row means. A graph names the behaviour the same
			way it names the data, so a surface drawn from nodes can open the
			same inspector the React one opens.
		*/
		inspectSymbol: (symbol: string) =>
			terminalStore.actions.openThesis(symbol),
	};
};
