import { encodeJsonBytes } from "#/lib/json-bytes";
import { Value } from "#/types/value";

/*
A mark tick names only the lots whose book moved. Those frames used to
arrive on the same "all" scope as the connect snapshot, so each tick
replaced the book and the open count with that sparse set. The book is
kept by lot id. A frame updates the lots it names and leaves the rest.
*/

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

const bookKey = (row: Record<string, unknown>, fallback: string) => {
	if (typeof row.id === "string" && row.id !== "") return row.id;

	if (typeof row.symbol === "string" && row.symbol !== "") {
		return `${row.virtual === true ? "virtual" : "paper"}:${row.symbol}`;
	}

	return fallback;
};

const lotValue = (
	row: Record<string, unknown>,
	source: Value,
	key: string,
): Value => {
	const role = source.role || "update";
	const prefix = `positions/${role}/${key}`;
	const value = new Value({
		id: source.id,
		epoch: source.epoch,
		tick: source.tick,
		seqIdx: source.seqIdx,
		timestamp: source.timestamp,
		origin: "positions",
		role,
		scope: key,
		prefix,
	});

	value.layers = [encodeJsonBytes(row)];
	value.index = [`${prefix}/position`];
	return value;
};

const applyPositionFrame = (next: Record<string, Value>, source: Value) => {
	const rows = framePositionRows(source);

	if (rows.length === 0) {
		if (source.scope !== "" && source.scope !== "all") {
			next[source.scope] = source;
		}
		return;
	}

	const tick = source.tick ?? 0n;

	for (const row of rows) {
		const key = bookKey(row, source.scope);
		if (key === "" || key === "all") continue;

		const prior = next[key];
		if (prior && (prior.tick ?? 0n) > tick) continue;

		next[key] =
			rows.length === 1 && source.scope === key
				? source
				: lotValue(row, source, key);
	}
};

export const mergePositionBook = (
	prev: Record<string, Value>,
	frame: Value,
): Record<string, Value> => {
	/*
	A connect snapshot is the whole book. Mark ticks are not: they name the
	lots that moved, even when they still arrive on the "all" scope.
	*/
	if (frame.role === "snapshot") {
		const next: Record<string, Value> = {};
		applyPositionFrame(next, frame);
		return next;
	}

	const next = { ...prev };
	const held = next.all;
	if (held) delete next.all;

	if (held && held !== frame) applyPositionFrame(next, held);
	applyPositionFrame(next, frame);
	delete next.all;

	return next;
};
