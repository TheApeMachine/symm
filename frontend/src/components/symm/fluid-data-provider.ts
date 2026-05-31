import type { FieldSnapshotEvent } from "#/lib/symm/events";
import {
	isFieldGridEvent,
	isFieldRowEvent,
	isFieldSnapshotEvent,
	type FluidSymbolRow,
} from "#/lib/symm/events";
import { buildFluidGrid } from "#/lib/symm/fluid-grid";

type FluidSink = (snapshot: FieldSnapshotEvent) => void;

/*
FluidDataProvider accumulates field rows and feeds the 3D surface chart.
*/
class FluidDataProviderImpl {
	private sink: FluidSink | null = null;
	private rows = new Map<string, FluidSymbolRow>();
	private latest: FieldSnapshotEvent | undefined;

	registerSink(sink: FluidSink) {
		this.sink = sink;

		if (this.latest !== undefined) {
			sink(this.latest);
		}

		return () => {
			if (this.sink === sink) {
				this.sink = null;
			}
		};
	}

	snapshot(): FieldSnapshotEvent | undefined {
		return this.latest;
	}

	private publish(ts: string) {
		const symbols = [...this.rows.values()];

		if (symbols.length === 0) {
			return;
		}

		const grid = buildFluidGrid(symbols);
		const snapshot: FieldSnapshotEvent = {
			event: "field_snapshot",
			ts,
			symbol_count: symbols.length,
			symbols,
			grid: {
				size: grid.heights.length,
				heights: grid.heights,
				min: grid.min,
				max: grid.max,
				filled_cells: grid.filledCells,
				outliers: {
					clipped_count: grid.outliers.clippedCount,
					clipped_at: grid.outliers.clippedAt,
					raw_max: grid.outliers.rawMax,
					raw_max_symbol: grid.outliers.rawMaxSymbol,
					display_max: grid.outliers.displayMax,
				},
			},
		};

		this.latest = snapshot;
		this.sink?.(snapshot);
	}

	ingest(raw: unknown) {
		if (isFieldRowEvent(raw)) {
			this.rows.set(raw.symbol, raw.row);
			this.publish(raw.ts);
			return;
		}

		if (isFieldSnapshotEvent(raw)) {
			this.rows.clear();

			for (const row of raw.symbols) {
				if (row.symbol) {
					this.rows.set(row.symbol, row);
				}
			}

			this.latest = raw;
			this.sink?.(raw);
			return;
		}

		if (!isFieldGridEvent(raw)) {
			return;
		}

		const symbols = [...this.rows.values()];
		const snapshot: FieldSnapshotEvent = {
			event: "field_snapshot",
			ts: raw.ts,
			symbol_count: symbols.length,
			symbols,
			grid: raw.grid,
		};

		this.latest = snapshot;
		this.sink?.(snapshot);
	}
}

const shared = createFluidDataProviderImpl();

export const createFluidDataProvider = () => createFluidDataProviderImpl();

function createFluidDataProviderImpl() {
	const impl = new FluidDataProviderImpl();

	return {
		registerSink: (sink: FluidSink) => impl.registerSink(sink),
		snapshot: () => impl.snapshot(),
		ingest: (raw: unknown) => impl.ingest(raw),
	};
}

export type FluidStore = ReturnType<typeof createFluidDataProvider>;

export const FluidDataProvider = shared;
