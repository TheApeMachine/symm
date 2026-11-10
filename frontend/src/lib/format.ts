/*
Wire scalars and display text — one place for the ceremonial coercions.
*/

/** Finite number from a wire scalar (number or numeric string). */
export const asFinite = (value: unknown): number | null => {
	if (typeof value === "number" && Number.isFinite(value)) return value;

	if (typeof value === "string" && value !== "") {
		const n = Number(value);
		return Number.isFinite(n) ? n : null;
	}

	return null;
};

/** BigInt from epoch/tick/timestamp wire fields. */
export const asBigInt = (value: unknown): bigint => {
	if (typeof value === "bigint") return value;

	if (typeof value === "number" && Number.isFinite(value)) {
		return BigInt(Math.trunc(value));
	}

	if (typeof value === "string" && value !== "") {
		try {
			return BigInt(value);
		} catch {
			return 0n;
		}
	}

	return 0n;
};

/** Signed USD. Adaptive digits unless `digits` is set. */
export const moneyText = (value: number, digits?: number): string => {
	const abs = Math.abs(value);
	const d = digits ?? (abs >= 100 ? 0 : abs >= 10 ? 1 : 2);
	const body = abs.toFixed(d);
	return value >= 0 ? `$+${body}` : `$-${body}`;
};

/** Signed percent. Adaptive digits unless `digits` is set. */
export const pctText = (value: number, digits?: number): string => {
	const d = digits ?? (Math.abs(value) >= 10 ? 0 : 1);
	const sign = value > 0 ? "+" : "";
	return `${sign}${value.toFixed(d)}%`;
};

/** Percent or em dash when missing. */
export const pctMark = (
	value: number | null | undefined,
	digits = 2,
): string =>
	value === null || value === undefined || !Number.isFinite(value)
		? "—"
		: pctText(value, digits);
