import type { FieldSnapshotEvent } from "#/lib/symm/events";
import {
	type FluidSymbolRow,
	isFieldGridEvent,
	isFieldRowEvent,
	isFieldSnapshotEvent,
} from "#/lib/symm/events";
import { buildFluidGrid } from "#/lib/symm/fluid-grid";

type FluidSink = (snapshot: FieldSnapshotEvent) => void;

/*
FluidChartAdapter merges incremental field_row wire frames into surface snapshots
the 3D chart update() accepts. Row assembly is wire-format adaptation, not chart state.
*/
class FluidChartAdapter {
	private sink: FluidSink | null = null;
	private rows = new Map<string, FluidSymbolRow>();

	registerSink(sink: FluidSink) {
		this.sink = sink;

		return () => {
			if (this.sink === sink) {
				this.sink = null;
			}
		};
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

		this.sink?.(snapshot);
	}
}

const shared = createFluidChartAdapter();

export const createFluidDataProvider = () => createFluidChartAdapter();

function createFluidChartAdapter() {
	const adapter = new FluidChartAdapter();

	return {
		registerSink: (sink: FluidSink) => adapter.registerSink(sink),
		ingest: (raw: unknown) => adapter.ingest(raw),
	};
}

export type FluidStore = ReturnType<typeof createFluidDataProvider>;

export const FluidDataProvider: FluidStore = shared;
