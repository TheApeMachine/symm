/*
Decode position lot rows from a Value-shaped frame's "position" layer.
Kept local after the positions collection was removed.
*/
export const framePositionRows = (frame: {
	lookup: (keys: string[]) => unknown[];
}): Record<string, unknown>[] => {
	const rows: Record<string, unknown>[] = [];

	for (const layer of frame.lookup(["position"])) {
		if (Array.isArray(layer)) {
			for (const row of layer) {
				if (row && typeof row === "object" && !Array.isArray(row)) {
					rows.push(row as Record<string, unknown>);
				}
			}
			continue;
		}

		if (layer && typeof layer === "object") {
			rows.push(layer as Record<string, unknown>);
		}
	}

	return rows;
};

export const comparePositionRows = (
	a: Record<string, unknown>,
	b: Record<string, unknown>,
) => {
	const symbol = String(a.symbol ?? "").localeCompare(String(b.symbol ?? ""));
	if (symbol !== 0) return symbol;

	const opened = String(a.opened_at ?? "").localeCompare(
		String(b.opened_at ?? ""),
	);
	if (opened !== 0) return opened;

	return String(a.id ?? "").localeCompare(String(b.id ?? ""));
};
