import { Store, useStore } from "@tanstack/react-store";

/*
GraphResultsState holds output results computed for nodes across graph IDs.
Only backend results for the requested graph revision are exposed.
*/
export type GraphResultsState = {
	resultsByGraphId: Record<
		string,
		{ version: string; results: Record<string, Record<string, unknown>> }
	>;
};

const EMPTY_RESULTS: Record<string, Record<string, unknown>> = {};

export const graphResultsStore = new Store<GraphResultsState>({
	resultsByGraphId: {},
});

export const setGraphResults = (
	graphId: string,
	version: string,
	results: Record<string, Record<string, unknown>>,
): void => {
	graphResultsStore.setState((previous: GraphResultsState) => {
		return {
			...previous,
			resultsByGraphId: {
				...previous.resultsByGraphId,
				[graphId]: { version, results },
			},
		};
	});
};

export const clearGraphResults = (graphId: string): void => {
	graphResultsStore.setState((previous: GraphResultsState) => {
		if (!(graphId in previous.resultsByGraphId)) {
			return previous;
		}

		const nextResults = { ...previous.resultsByGraphId };
		delete nextResults[graphId];

		return {
			...previous,
			resultsByGraphId: nextResults,
		};
	});
};

export const getGraphResults = (
	graphId: string,
	version: string,
): Record<string, Record<string, unknown>> => {
	const snapshot = graphResultsStore.state.resultsByGraphId[graphId];
	return snapshot?.version === version ? snapshot.results : EMPTY_RESULTS;
};

export const useGraphResults = (
	graphId: string,
	version: string,
): Record<string, Record<string, unknown>> =>
	useStore(graphResultsStore, (state) => {
		const snapshot = state.resultsByGraphId[graphId];
		return snapshot?.version === version ? snapshot.results : EMPTY_RESULTS;
	});
