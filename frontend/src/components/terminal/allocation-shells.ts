/*
createAllocRow builds one cross-section ladder shell for a symbol.
*/
const createAllocRow = (symbol: string): HTMLElement => {
	const row = document.createElement("div");
	row.dataset.allocRow = symbol;
	row.dataset.symbol = symbol;
	row.className =
		"flex items-center gap-[9px] border-(--line) border-b py-[7px]";
	row.style.display = "none";

	const name = document.createElement("span");
	name.dataset.alloc = "name";
	name.className =
		"w-[58px] shrink-0 font-mono text-[11px] font-semibold";
	name.textContent = symbol.split("/")[0] ?? symbol;
	row.append(name);

	const track = document.createElement("div");
	track.className = "relative h-[18px] flex-1";

	const baseline = document.createElement("div");
	baseline.className = "absolute top-2 right-0 left-0 h-px bg-(--line)";

	const medianMark = document.createElement("div");
	medianMark.dataset.alloc = "median-mark";
	medianMark.className = "absolute top-px bottom-px w-px bg-(--f4)";

	const thresholdMark = document.createElement("div");
	thresholdMark.dataset.alloc = "threshold-mark";
	thresholdMark.className =
		"absolute top-0 bottom-0 w-px bg-[color-mix(in_srgb,var(--acc)_70%,transparent)]";

	const edgeBar = document.createElement("div");
	edgeBar.dataset.alloc = "edge-bar";
	edgeBar.className =
		"absolute top-[7px] h-[3px] rounded-sm bg-(--acc)";

	const dot = document.createElement("div");
	dot.dataset.alloc = "dot";
	dot.className =
		"absolute top-1 h-[9px] w-[9px] rounded-full border border-(--sunken)";
	dot.style.marginLeft = "-4.5px";

	track.append(baseline, medianMark, thresholdMark, edgeBar, dot);
	row.append(track);

	for (const [field, width, tone] of [
		["edge", "w-[52px] shrink-0 text-right font-mono text-[10px]", ""],
		[
			"share",
			"w-[42px] shrink-0 text-right font-mono text-[10px] text-(--f2)",
			"",
		],
		[
			"notional",
			"w-[74px] shrink-0 text-right font-mono text-[10.5px]",
			"",
		],
	] as const) {
		const cell = document.createElement("span");
		cell.dataset.alloc = field;
		cell.className = width;

		if (tone !== "") {
			cell.className = `${cell.className} ${tone}`;
		}

		row.append(cell);
	}

	return row;
};

/*
createAllocSize builds one position-sizing shell for a symbol.
*/
const createAllocSize = (symbol: string): HTMLElement => {
	const sizing = document.createElement("div");
	sizing.dataset.allocSize = symbol;
	sizing.dataset.symbol = symbol;
	sizing.style.display = "none";

	const head = document.createElement("div");
	head.className = "mb-1 flex items-center justify-between";

	const label = document.createElement("span");
	label.className = "font-mono text-[11px] font-semibold text-(--f1)";
	label.textContent = symbol.split("/")[0] ?? symbol;

	const notional = document.createElement("span");
	notional.dataset.alloc = "size-notional";
	notional.className = "font-mono text-[10.5px] text-(--acc)";

	head.append(label, notional);
	sizing.append(head);

	const barRow = document.createElement("div");
	barRow.className = "flex items-center gap-2";

	const track = document.createElement("div");
	track.className =
		"h-1.5 flex-1 overflow-hidden rounded-[2px] bg-(--line)";

	const fill = document.createElement("div");
	fill.dataset.alloc = "size-fill";
	fill.className = "h-full bg-(--acc)";
	fill.style.width = "0%";
	track.append(fill);

	const share = document.createElement("span");
	share.dataset.alloc = "size-share";
	share.className =
		"w-10 shrink-0 text-right font-mono text-[10px] text-(--f2)";

	barRow.append(track, share);
	sizing.append(barRow);

	return sizing;
};

/*
syncAllocShells creates or removes ladder and sizing shells when symbols appear.
*/
export const syncAllocShells = (
	root: HTMLElement,
	symbols: string[],
): void => {
	const rowHost = root.querySelector("[data-alloc-host='rows']");
	const sizeHost = root.querySelector("[data-alloc-host='sizes']");

	if (!(rowHost instanceof HTMLElement) || !(sizeHost instanceof HTMLElement)) {
		return;
	}

	const next = new Set(symbols);
	const rowOrder: HTMLElement[] = [];

	for (const symbol of symbols) {
		const escaped = CSS.escape(symbol);
		const existing = root.querySelector(`[data-alloc-row='${escaped}']`);
		let row: HTMLElement;

		if (existing instanceof HTMLElement) {
			row = existing;
		} else {
			row = createAllocRow(symbol);
			rowHost.append(row);
		}

		rowOrder.push(row);
	}

	for (const row of rowHost.querySelectorAll("[data-alloc-row]")) {
		if (!(row instanceof HTMLElement)) {
			continue;
		}

		const symbol = row.dataset.allocRow;

		if (symbol === undefined || next.has(symbol)) {
			continue;
		}

		row.remove();
	}

	const rowMatches =
		rowOrder.length ===
			rowHost.querySelectorAll("[data-alloc-row]").length &&
		rowOrder.every(
			(row, index) =>
				rowHost.querySelectorAll("[data-alloc-row]")[index] === row,
		);

	if (!rowMatches) {
		const waiting = rowHost.querySelector("[data-alloc='waiting']");
		rowHost.replaceChildren(...(waiting ? [waiting] : []), ...rowOrder);
	}

	const sizeOrder: HTMLElement[] = [];

	for (const symbol of symbols) {
		const escaped = CSS.escape(symbol);
		const existing = root.querySelector(`[data-alloc-size='${escaped}']`);
		let sizing: HTMLElement;

		if (existing instanceof HTMLElement) {
			sizing = existing;
		} else {
			sizing = createAllocSize(symbol);
			sizeHost.append(sizing);
		}

		sizeOrder.push(sizing);
	}

	for (const sizing of sizeHost.querySelectorAll("[data-alloc-size]")) {
		if (!(sizing instanceof HTMLElement)) {
			continue;
		}

		const symbol = sizing.dataset.allocSize;

		if (symbol === undefined || next.has(symbol)) {
			continue;
		}

		sizing.remove();
	}

	const sizingEmpty = sizeHost.querySelector("[data-alloc='sizing-empty']");
	const sizeMatches =
		sizeOrder.length ===
			sizeHost.querySelectorAll("[data-alloc-size]").length &&
		sizeOrder.every(
			(sizing, index) =>
				sizeHost.querySelectorAll("[data-alloc-size]")[index] === sizing,
		);

	if (!sizeMatches) {
		sizeHost.replaceChildren(
			...(sizingEmpty ? [sizingEmpty] : []),
			...sizeOrder,
		);
	}
};
