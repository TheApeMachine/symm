import type { Measurement } from "#/collections/types";

export type MeterParts = {
	cell: HTMLElement;
	value: HTMLElement;
	fill: HTMLElement;
};

/*
mergeInspectorMetrics unions compact metric maps across every DRAW row for the
inspected source. Hawkes publishes live intensity and fit-parameter epochs as
separate source×symbol×at groups; reconciling meters against each row alone
deletes the other group's cells every tick.
*/
export const mergeInspectorMetrics = (
	rows: Measurement[],
	source: string,
	focusSymbol: string,
): Map<string, number> => {
	const merged = new Map<string, number>();

	for (const row of rows) {
		if (row.source !== source) {
			continue;
		}

		if (focusSymbol !== "" && row.symbol !== focusSymbol) {
			continue;
		}

		const entries = Object.entries(row.metrics ?? {});

		for (const [key, entry] of entries) {
			const numeric = entry.normalized ?? entry.raw;
			merged.set(key, Number.isFinite(numeric) ? numeric : 0);
		}
	}

	return merged;
};

/*
paintInspectorMeters creates meter shells once and patches value/fill in place.
Signals emit a complete metric set every tick, so keys absent from entries are
stale and removed. Shells also clear when the inspected source changes.
*/
export const paintInspectorMeters = (
	host: HTMLElement,
	meters: Map<string, MeterParts>,
	entries: Map<string, number>,
	headline?: string,
) => {
	const sortedEntries = [...entries.entries()].sort(([left], [right]) =>
		left.localeCompare(right),
	);

	for (const [key, entry] of sortedEntries) {
		let meter = meters.get(key);

		if (meter === undefined) {
			const cell = document.createElement("div");
			cell.dataset.metric = key;
			cell.setAttribute("role", "progressbar");
			cell.setAttribute("aria-valuemin", "0");
			cell.setAttribute("aria-valuemax", "100");

			const header = document.createElement("div");
			header.className = "mb-1 flex justify-between font-mono text-[9px]";
			const label = document.createElement("span");
			const valueEl = document.createElement("span");
			label.className = "text-(--f3)";
			valueEl.className = "text-(--f1)";
			label.textContent = key.replaceAll("_", " ");
			header.append(label, valueEl);

			const track = document.createElement("div");
			track.className =
				"h-1 overflow-hidden rounded-[2px] bg-(--line) [--meter-tone:var(--info)]";
			const fill = document.createElement("div");
			fill.className = "h-full bg-(--meter-tone)";
			track.append(fill);
			cell.append(header, track);
			host.append(cell);

			meter = { cell, value: valueEl, fill };
			meters.set(key, meter);
		}

		const fillPercent = Math.max(0, Math.min(100, entry * 100));

		meter.value.textContent = entry.toPrecision(4);
		meter.fill.style.width = `${fillPercent}%`;
		meter.cell.setAttribute(
			"aria-valuenow",
			String(Math.round(fillPercent)),
		);
		if (headline !== undefined) {
			meter.fill.className =
				key === headline ? "h-full bg-(--warning)" : "h-full bg-(--info)";
		}
	}

	for (const [key, meter] of meters) {
		if (entries.has(key)) {
			continue;
		}

		meter.cell.remove();
		meters.delete(key);
	}

	for (const [index, [key]] of sortedEntries.entries()) {
		const meter = meters.get(key);
		if (
			meter &&
			typeof host.insertBefore === "function" &&
			host.children?.[index] !== meter.cell
		) {
			host.insertBefore(meter.cell, host.children[index] || null);
		}
	}
};
