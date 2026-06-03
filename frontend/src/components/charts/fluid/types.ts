export type FluidSymbolRow = {
	symbol: string;
	change_pct: number;
	vol: number;
	div: number;
	vort: number;
	turb: number;
	visc: number;
	re: number;
};

export type FieldSnapshotEvent = {
	type: "fluid";
	ts: string;
	symbol_count: number;
	symbols: FluidSymbolRow[];
};
