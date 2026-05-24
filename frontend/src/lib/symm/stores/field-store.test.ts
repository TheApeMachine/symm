import { describe, expect, it } from "vitest";

import {
	applyFieldAggregate,
	applyFieldGrid,
	applyFieldRow,
	buildFieldSnapshot,
	fieldStore,
} from "#/lib/symm/stores/field-store";

describe("fieldStore", () => {
	it("merges field_row and field_aggregate incrementally", () => {
		fieldStore.setState({ rows: {}, symbolCount: 0 });

		applyFieldRow({
			symbol: "BTC/EUR",
			change_pct: 1.2,
			vol: 0.5,
			div: 0.1,
			vort: 0.2,
			turb: 0.3,
			visc: 0.4,
			re: 42,
		});

		applyFieldAggregate(1, {
			re: 42,
			vort: 0.2,
			div: 0.1,
			turb: 0.3,
			visc: 0.4,
		});

		const snapshot = buildFieldSnapshot(fieldStore.state);

		expect(snapshot?.symbol_count).toBe(1);
		expect(snapshot?.field.re).toBe(42);
		expect(snapshot?.symbols).toHaveLength(1);
	});

	it("applies field_grid without clearing rows", () => {
		fieldStore.setState({
			rows: {
				"ETH/EUR": {
					symbol: "ETH/EUR",
					change_pct: 0,
					vol: 1,
					div: 0,
					vort: 0,
					turb: 0,
					visc: 0,
					re: 10,
				},
			},
			symbolCount: 1,
		});

		applyFieldGrid({
			size: 2,
			heights: [
				[0, 1],
				[1, 0],
			],
			min: 0,
			max: 1,
			filled_cells: 2,
			outliers: {
				clipped_count: 0,
				clipped_at: 0,
				raw_max: 1,
				display_max: 1,
			},
		});

		const snapshot = buildFieldSnapshot(fieldStore.state);

		expect(snapshot?.grid?.filled_cells).toBe(2);
		expect(snapshot?.symbols).toHaveLength(1);
	});
});
