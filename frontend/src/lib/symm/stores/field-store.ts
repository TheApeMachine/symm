import { createStore } from "@tanstack/react-store";

import type {
	FieldAggregate,
	FieldSnapshotEvent,
	FluidDisplayEvent,
	FluidGridPayload,
	FluidSymbolRow,
} from "#/lib/symm/events";

export type FieldStoreState = {
	rows: Record<string, FluidSymbolRow>;
	aggregate?: FieldAggregate;
	symbolCount: number;
	grid?: FluidGridPayload;
	fluidDisplay?: FluidDisplayEvent;
};

export const fieldStore = createStore<FieldStoreState>({
	rows: {},
	symbolCount: 0,
});

export const buildFieldSnapshot = (
	state: FieldStoreState,
): FieldSnapshotEvent | undefined => {
	const symbols = Object.values(state.rows);

	if (symbols.length === 0 && !state.grid) {
		return undefined;
	}

	return {
		event: "field_snapshot",
		ts: new Date().toISOString(),
		symbol_count: state.symbolCount || symbols.length,
		field: state.aggregate ?? { re: 0, vort: 0, div: 0, turb: 0, visc: 0 },
		symbols,
		grid: state.grid,
	};
};

export const applyFieldRow = (row: FluidSymbolRow): void => {
	fieldStore.setState((state) => ({
		...state,
		rows: {
			...state.rows,
			[row.symbol]: row,
		},
	}));
};

export const applyFieldAggregate = (
	symbolCount: number,
	field: FieldAggregate,
): void => {
	fieldStore.setState((state) => ({
		...state,
		symbolCount,
		aggregate: field,
	}));
};

export const applyFieldGrid = (grid: FluidGridPayload): void => {
	fieldStore.setState((state) => ({
		...state,
		grid,
	}));
};

export const applyFieldSnapshot = (snapshot: FieldSnapshotEvent): void => {
	const rows: Record<string, FluidSymbolRow> = {};

	for (const row of snapshot.symbols ?? []) {
		rows[row.symbol] = row;
	}

	fieldStore.setState((state) => ({
		...state,
		rows,
		symbolCount: snapshot.symbol_count,
		aggregate: snapshot.field,
		grid: snapshot.grid ?? state.grid,
	}));
};

export const applyFluidDisplay = (display: FluidDisplayEvent) => {
	fieldStore.setState((state) => ({ ...state, fluidDisplay: display }));
};
