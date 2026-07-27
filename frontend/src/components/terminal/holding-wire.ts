import type { Holding } from "#/collections/types";

const decimalKeys = [
	"qty",
	"entry_price",
	"entry_fee",
	"exit_price",
	"exit_fee",
	"mark",
	"pnl",
	"return_pct",
	"peak_price",
	"stop_price",
	"stop_return",
	"peak_return",
	"momentum_health",
	"stagnation_health",
] as const;

/*
holdingRows normalizes the backend holdings frame without changing its wire
schema. Balance publishes a symbol-keyed map and Kraken decimals may arrive as
JSON strings, so painters consume number-ready copies while preserving the flat
backend payload shape.
*/
export const holdingRows = (value: unknown): Holding[] => {
	const rows = (
		Array.isArray(value)
			? value
			: value !== null && typeof value === "object"
				? Object.values(value as Record<string, Holding>)
				: value != null
					? [value]
					: []
	) as Holding[];

	return rows.map((holding) => {
		const out = { ...holding } as Record<string, unknown>;

		for (const key of decimalKeys) {
			const value = out[key];

			if (typeof value === "string" && value.trim() !== "") {
				out[key] = Number(value);
			}
		}

		return out as Holding;
	}).sort((left, right) => left.symbol.localeCompare(right.symbol));
};
