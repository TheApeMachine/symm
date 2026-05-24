import type { FieldAggregate, FluidSymbolRow } from "#/lib/symm/events";

export function formatFluidScalar(value: number): string {
	if (!Number.isFinite(value)) {
		return "—";
	}

	const magnitude = Math.abs(value);

	if (magnitude === 0) {
		return "0";
	}

	if (magnitude < 0.01) {
		return value.toFixed(4);
	}

	return value.toFixed(2);
}

function median(values: number[]): number {
	if (values.length === 0) {
		return 0;
	}

	const sorted = [...values].sort((left, right) => left - right);
	const mid = Math.floor(sorted.length / 2);

	if (sorted.length % 2 === 0) {
		return (sorted[mid - 1] + sorted[mid]) / 2;
	}

	return sorted[mid];
}

/** Prefer cross-section aggregate; fall back to symbol medians for header display. */
export function headerFieldMetrics(
	field: FieldAggregate | undefined,
	symbols: FluidSymbolRow[] | undefined,
): FieldAggregate {
	if (!field) {
		return { re: 0, vort: 0, div: 0, turb: 0, visc: 0 };
	}

	const sampled = (symbols ?? []).filter((row) => row.vol > 0 || row.visc > 0);

	if (sampled.length === 0) {
		return field;
	}

	const pick = (aggregate: number, values: number[]): number => {
		if (Math.abs(aggregate) >= 1e-4) {
			return aggregate;
		}

		return median(values.filter((value) => Number.isFinite(value)));
	};

	return {
		re: pick(
			field.re,
			sampled.map((row) => row.re),
		),
		vort: pick(
			field.vort,
			sampled.map((row) => row.vort),
		),
		div: pick(
			field.div,
			sampled.map((row) => row.div),
		),
		turb: pick(
			field.turb,
			sampled.map((row) => row.turb),
		),
		visc: pick(
			field.visc,
			sampled.map((row) => row.visc),
		),
	};
}
