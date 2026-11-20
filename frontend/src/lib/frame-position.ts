export const comparePositionRows = (
	a: { id?: string; symbol?: string; opened_at?: string },
	b: { id?: string; symbol?: string; opened_at?: string },
) => {
	const symbol = String(a.symbol ?? "").localeCompare(String(b.symbol ?? ""));
	if (symbol !== 0) return symbol;

	const opened = String(a.opened_at ?? "").localeCompare(
		String(b.opened_at ?? ""),
	);
	if (opened !== 0) return opened;

	return String(a.id ?? "").localeCompare(String(b.id ?? ""));
};
