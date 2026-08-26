/*
num formats a nullable numeric (or numeric-string) value to a fixed-digit
string, falling back to an em dash for absent or non-numeric input.
*/
export const num = (v: unknown, d: number): string =>
	typeof v === "number"
		? v.toFixed(d)
		: typeof v === "string" && v.trim() !== "" && Number.isFinite(Number(v))
			? Number(v).toFixed(d)
			: "—";
