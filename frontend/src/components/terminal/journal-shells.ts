import { cn } from "#/lib/utils";
import { LIFECYCLE_STAGES } from "#/types/thesis";
import { badgeVariants } from "@/components/ui/badge";
import { panelVariants } from "@/components/ui/panel";

const panelShell = (className: string): string =>
	cn(panelVariants({ variant: "surface", size: "bare" }), className);

const createLifecycleTrackShell = (symbol: string): HTMLElement => {
	const track = document.createElement("div");
	track.dataset.lifecycleTrack = symbol;
	track.className =
		"rounded border border-(--line) bg-(--surface) px-3 py-2.5";

	const head = document.createElement("div");
	head.className = "mb-2 flex items-center justify-between gap-2";

	const label = document.createElement("span");
	label.dataset.lifecycle = "symbol";
	label.className = "font-mono font-semibold text-[12px] text-(--f1)";
	label.textContent = symbol;

	const badge = document.createElement("span");
	badge.dataset.lifecycle = "badge";
	badge.className = cn(badgeVariants({ variant: "info", size: "xs" }));
	badge.textContent = "observing";
	head.append(label, badge);

	const stages = document.createElement("div");
	stages.className = "flex flex-wrap gap-1";

	for (const stage of LIFECYCLE_STAGES) {
		const node = document.createElement("span");
		node.dataset.lifecycleStage = stage;
		node.title = stage.replaceAll("_", " ");
		node.className =
			"rounded-[2px] border px-1 py-px font-mono text-[8px] uppercase tracking-wide";
		node.textContent = stage.split("_")[0] ?? stage;
		stages.append(node);
	}

	track.append(head, stages);

	return track;
};

const createLifecycleButton = (
	symbol: string,
	onSelect: (symbol: string) => void,
): HTMLButtonElement => {
	const button = document.createElement("button");
	button.type = "button";
	button.dataset.symbol = symbol;
	button.dataset.journalLifecycle = symbol;
	button.className = "cursor-pointer text-left";
	button.addEventListener("click", () => onSelect(symbol));
	button.append(createLifecycleTrackShell(symbol));

	return button;
};

const createHoldingCard = (key: string): HTMLElement => {
	const card = document.createElement("div");
	card.dataset.holding = key;
	card.className = panelShell(
		"grid grid-cols-[1fr_auto] items-start gap-3 px-3 py-2.5",
	);
	card.style.display = "none";

	const body = document.createElement("div");
	body.className = "min-w-0";

	const symbol = document.createElement("div");
	symbol.dataset.journal = "holding-symbol";
	symbol.className = "font-mono font-semibold text-[12px] text-(--f1)";

	const meta = document.createElement("div");
	meta.dataset.journal = "holding-meta";
	meta.className = "mt-0.5 truncate font-mono text-[10px] text-(--f3)";
	body.append(symbol, meta);

	const status = document.createElement("span");
	status.dataset.journal = "holding-status";
	card.append(body, status);

	return card;
};

const createJournalCard = (
	key: string,
	onSelect: (key: string) => void,
): HTMLElement => {
	const card = document.createElement("button");
	card.type = "button";
	card.dataset.journalEntry = key;
	card.className = cn(panelShell("w-full px-3 py-2.5 text-left"), "cursor-pointer");
	card.style.display = "none";
	card.addEventListener("click", () => onSelect(key));

	const head = document.createElement("div");
	head.className = "mb-2 flex items-start justify-between gap-2";

	const title = document.createElement("div");
	title.className = "min-w-0";

	const symbol = document.createElement("div");
	symbol.dataset.journal = "entry-symbol";
	symbol.className = "font-mono font-semibold text-[12px] text-(--f1)";

	const meta = document.createElement("div");
	meta.dataset.journal = "entry-meta";
	meta.className = "mt-0.5 truncate font-mono text-[10px] text-(--f4)";
	title.append(symbol, meta);

	const badge = document.createElement("span");
	badge.dataset.journal = "entry-lifecycle";
	head.append(title, badge);

	const reason = document.createElement("div");
	reason.dataset.journal = "entry-reason";
	reason.className = "font-medium text-[11px] text-(--f2)";

	const detail = document.createElement("div");
	detail.dataset.journal = "entry-detail";
	detail.className = "mt-1 font-mono text-[9.5px] text-(--f3)";

	const graph = document.createElement("div");
	graph.dataset.journal = "entry-graph";
	graph.className = "mt-1 font-mono text-[9px] text-(--f4)";

	card.append(head, reason, detail, graph);

	return card;
};

const createFindingCard = (key: string): HTMLElement => {
	const card = document.createElement("div");
	card.dataset.finding = key;
	card.className = panelShell("px-3 py-2.5");
	card.style.display = "none";

	const head = document.createElement("div");
	head.className = "mb-2 flex items-center justify-between gap-2";

	const component = document.createElement("span");
	component.dataset.journal = "finding-component";
	component.className = cn(
		badgeVariants({ variant: "warning", size: "xs" }),
	);

	const unc = document.createElement("span");
	unc.dataset.journal = "finding-unc";
	unc.className = "font-mono text-[9px] text-(--f4)";
	head.append(component, unc);

	const condition = document.createElement("div");
	condition.dataset.journal = "finding-condition";
	condition.className = "font-medium text-[12px] text-(--f1)";

	const effectBlock = document.createElement("div");
	effectBlock.className = "mt-2";

	const effectHeader = document.createElement("div");
	effectHeader.className =
		"mb-1 flex justify-between font-mono text-[9px] text-(--f4)";

	const effectLabel = document.createElement("span");
	effectLabel.textContent = "estimated effect";

	const effectValue = document.createElement("span");
	effectValue.dataset.journal = "finding-effect";
	effectValue.className = "text-(--f1)";
	effectHeader.append(effectLabel, effectValue);

	const effectTrack = document.createElement("div");
	effectTrack.className =
		"h-[5px] overflow-hidden rounded-[3px] bg-(--line)";

	const effectFill = document.createElement("div");
	effectFill.dataset.journal = "finding-fill";
	effectFill.className = "h-full";
	effectFill.style.width = "0%";
	effectTrack.append(effectFill);
	effectBlock.append(effectHeader, effectTrack);

	const adjustment = document.createElement("div");
	adjustment.dataset.journal = "finding-adjustment";
	adjustment.className = "mt-2 font-mono text-[10px] text-(--acc)";
	adjustment.style.display = "none";

	const evidence = document.createElement("ul");
	evidence.dataset.journal = "finding-evidence";
	evidence.className =
		"mt-2 flex flex-col gap-1 font-mono text-[9.5px] text-(--f3)";

	const validate = document.createElement("div");
	validate.dataset.journal = "finding-validate";
	validate.className = "mt-2 font-mono text-[9px] text-(--f4)";

	card.append(
		head,
		condition,
		effectBlock,
		adjustment,
		evidence,
		validate,
	);

	return card;
};

const syncHostRows = (
	host: HTMLElement | null,
	keys: string[],
	create: (key: string) => HTMLElement,
	marker: string,
): void => {
	if (host === null) {
		return;
	}

	const next = new Set(keys);
	const ordered: HTMLElement[] = [];

	for (const key of keys) {
		const escaped = CSS.escape(key);
		const existing = host.querySelector(`[${marker}='${escaped}']`);
		let row: HTMLElement;

		if (existing instanceof HTMLElement) {
			row = existing;
		} else {
			row = create(key);
			host.append(row);
		}

		ordered.push(row);
	}

	for (const row of host.querySelectorAll(`[${marker}]`)) {
		if (!(row instanceof HTMLElement)) {
			continue;
		}

		const key = row.getAttribute(marker);

		if (key === null || next.has(key)) {
			continue;
		}

		row.remove();
	}

	const orderMatches =
		ordered.length === host.querySelectorAll(`[${marker}]`).length &&
		ordered.every(
			(row, index) => host.querySelectorAll(`[${marker}]`)[index] === row,
		);

	if (!orderMatches) {
		const empty = host.querySelector("[data-journal$='-empty']");
		host.replaceChildren(...(empty ? [empty] : []), ...ordered);
	}
};

const syncLifecycleShells = (
	root: HTMLElement,
	symbols: string[],
	onSelect: (symbol: string) => void,
): void => {
	const host = root.querySelector("[data-journal-host='lifecycle']");

	if (!(host instanceof HTMLElement)) {
		return;
	}

	const next = new Set(symbols);
	const ordered: HTMLElement[] = [];

	for (const symbol of symbols) {
		const escaped = CSS.escape(symbol);
		const existing = root.querySelector(
			`[data-journal-lifecycle='${escaped}']`,
		);
		let button: HTMLButtonElement;

		if (existing instanceof HTMLButtonElement) {
			button = existing;
		} else {
			button = createLifecycleButton(symbol, onSelect);
			host.append(button);
		}

		ordered.push(button);
	}

	for (const button of host.querySelectorAll("[data-journal-lifecycle]")) {
		if (!(button instanceof HTMLButtonElement)) {
			continue;
		}

		const symbol = button.dataset.journalLifecycle;

		if (symbol === undefined || next.has(symbol)) {
			continue;
		}

		button.remove();
	}

	const lifecycleEmpty = host.querySelector("[data-journal='lifecycle-empty']");
	const orderMatches =
		ordered.length ===
			host.querySelectorAll("[data-journal-lifecycle]").length &&
		ordered.every(
			(button, index) =>
				host.querySelectorAll("[data-journal-lifecycle]")[index] ===
				button,
		);

	if (!orderMatches) {
		host.replaceChildren(
			...(lifecycleEmpty ? [lifecycleEmpty] : []),
			...ordered,
		);
	}
};

/*
syncJournalShells creates or reorders lifecycle, journal, and finding shells
when journal keys change so paintJournalSurface only writes values in place.
*/
export const syncJournalShells = (
	root: HTMLElement,
	input: {
		symbols: string[];
		onLifecycleSelect: (symbol: string) => void;
		onJournalSelect: (key: string) => void;
		journalKeys: string[];
		findingKeys: string[];
	},
): void => {
	syncLifecycleShells(root, input.symbols, input.onLifecycleSelect);
	syncHostRows(
		root.querySelector("[data-journal-host='journal']"),
		input.journalKeys,
		(key) => createJournalCard(key, input.onJournalSelect),
		"data-journal-entry",
	);
	syncHostRows(
		root.querySelector("[data-journal-host='findings']"),
		input.findingKeys,
		createFindingCard,
		"data-finding",
	);
};
