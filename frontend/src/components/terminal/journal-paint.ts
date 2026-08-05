import type { Thesis } from "#/collections/types";
import { fixed } from "#/components/terminal/decision-format";
import { syncJournalShells } from "#/components/terminal/journal-shells";
import { paintLifecycleTrack } from "#/components/terminal/lifecycle-track";
import { cn } from "#/lib/utils";
import type { Finding, LifecycleState } from "#/types/thesis";
import { badgeVariants } from "@/components/ui/badge";

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

export const findingKey = (finding: Finding): string =>
	`${finding.symbol}:${finding.component}:${finding.condition}`;

export const journalEntryKey = (entry: Thesis): string => {
	const symbol =
		Object.keys(entry.lifecycle ?? {})[0] ??
		Object.keys(entry.holdings ?? {})[0] ??
		"";
	const lifecycle = symbol === "" ? "" : (entry.lifecycle?.[symbol] ?? "");

	return `${symbol}:${entry.tick}:${lifecycle}:${entry.at}`;
};

const paintJournalEntryRow = (row: HTMLElement, entry: Thesis): void => {
	const symbol =
		Object.keys(entry.lifecycle ?? {})[0] ??
		Object.keys(entry.holdings ?? {})[0] ??
		"";
	const lifecycleState = symbol === "" ? "" : (entry.lifecycle?.[symbol] ?? "");
	const decision =
		(entry.decisions ?? []).find((item) => item.symbol === symbol) ??
		entry.decisions?.[0];
	const holding = symbol === "" ? undefined : entry.holdings?.[symbol];
	const findings = (entry.findings ?? []).filter(
		(finding) => finding.symbol === symbol,
	);
	const graph = entry.graphs?.categories;

	setText(row.querySelector("[data-journal='entry-symbol']"), symbol);
	setText(
		row.querySelector("[data-journal='entry-meta']"),
		`${entry.at.slice(11, 19)} · tick ${entry.tick}`,
	);

	const lifecycle = row.querySelector("[data-journal='entry-lifecycle']");

	if (lifecycle instanceof HTMLElement) {
		lifecycle.textContent = lifecycleState;
		lifecycle.className = cn(badgeVariants({ variant: "info", size: "xs" }));
	}

	setText(
		row.querySelector("[data-journal='entry-reason']"),
		decision?.reason ||
			decision?.cause ||
			decision?.action ||
			"state transition",
	);
	const findingsCount = findings.length;
	const graphText =
		graph == null
			? "graph —"
			: `graph ${graph.nodes.length} nodes · ${graph.edges.length} edges`;

	setText(
		row.querySelector("[data-journal='entry-detail']"),
		holding == null
			? `decision ${decision?.action ?? "—"} · findings ${findingsCount}`
			: [
					`qty ${fixed(holding.qty)}`,
					`entry ${fixed(holding.entry_price)}`,
					`mark ${fixed(holding.mark)}`,
					`pnl ${fixed(holding.pnl)}`,
					`findings ${findingsCount}`,
				]
					.filter(Boolean)
					.join(" · "),
	);
	setText(row.querySelector("[data-journal='entry-graph']"), graphText);
};

const paintFindingCard = (card: HTMLElement, finding: Finding): void => {
	setText(
		card.querySelector("[data-journal='finding-component']"),
		finding.component,
	);
	setText(
		card.querySelector("[data-journal='finding-unc']"),
		`±${finding.uncertainty.toFixed(3)} unc`,
	);
	setText(
		card.querySelector("[data-journal='finding-condition']"),
		finding.condition,
	);
	setText(
		card.querySelector("[data-journal='finding-effect']"),
		finding.estimatedEffect.toFixed(4),
	);

	const fill = card.querySelector("[data-journal='finding-fill']");

	if (fill instanceof HTMLElement) {
		fill.style.width = `${Math.min(100, Math.round(Math.abs(finding.estimatedEffect) * 100))}%`;
		fill.style.background =
			finding.estimatedEffect >= 0 ? "var(--up)" : "var(--down)";
	}

	const adjustment = card.querySelector("[data-journal='finding-adjustment']");

	if (adjustment instanceof HTMLElement) {
		const text = finding.proposedAdjustment
			? `→ ${finding.proposedAdjustment}`
			: "";

		adjustment.textContent = text;
		adjustment.style.display = text === "" ? "none" : "";
	}

	const evidence = card.querySelector("[data-journal='finding-evidence']");

	if (evidence instanceof HTMLElement) {
		while (evidence.childElementCount > finding.evidence.length) {
			evidence.lastElementChild?.remove();
		}

		for (const [index, line] of finding.evidence.entries()) {
			let item = evidence.children[index] as HTMLLIElement | undefined;

			if (item === undefined) {
				item = document.createElement("li");
				item.className = "border-(--line) border-l pl-2";
				evidence.append(item);
			}

			if (item.textContent !== line) {
				item.textContent = line;
			}
		}
	}

	setText(
		card.querySelector("[data-journal='finding-validate']"),
		`validate · ${finding.requiredValidation}`,
	);
};

const paintLifecycleSelection = (
	root: HTMLElement,
	activeSymbol: string | null,
): void => {
	for (const button of root.querySelectorAll("[data-journal-lifecycle]")) {
		if (!(button instanceof HTMLButtonElement)) {
			continue;
		}

		const selected = button.dataset.journalLifecycle === activeSymbol;

		button.className = cn(
			"cursor-pointer text-left",
			selected &&
				"rounded ring-1 ring-[color-mix(in_srgb,var(--acc)_35%,transparent)]",
		);
	}
};

/*
paintJournalSurface writes lifecycle rails, persisted journal entries, and
postmortem findings into mounted shells so websocket cadence never re-renders
JournalSurface.
*/
export const paintJournalSurface = (
	root: HTMLElement | null,
	input: {
		activeSymbol: string | null;
		findings: Finding[];
		findingKeys: string[];
		entries: Thesis[];
		journalKeys: string[];
		lifecycleBySymbol: Record<string, string>;
		onJournalSelect: (key: string) => void;
		onLifecycleSelect: (symbol: string) => void;
		online: boolean;
		symbols: string[];
	},
): void => {
	if (root === null) {
		return;
	}

	const {
		activeSymbol,
		entries,
		findings,
		findingKeys,
		journalKeys,
		lifecycleBySymbol,
		onJournalSelect,
		onLifecycleSelect,
		online,
		symbols,
	} = input;

	syncJournalShells(root, {
		symbols,
		onJournalSelect,
		onLifecycleSelect,
		journalKeys,
		findingKeys,
	});
	paintLifecycleSelection(root, activeSymbol);

	setText(
		root.querySelector("[data-journal='lifecycle-meta']"),
		`${symbols.length} symbol${symbols.length === 1 ? "" : "s"}`,
	);

	const lifecycleEmpty = root.querySelector("[data-journal='lifecycle-empty']");

	if (lifecycleEmpty instanceof HTMLElement) {
		lifecycleEmpty.style.display = symbols.length === 0 ? "" : "none";
		lifecycleEmpty.textContent = online
			? "no active lifecycle"
			: "waiting for lifecycle frames";
	}

	for (const symbol of symbols) {
		paintLifecycleTrack(
			root.querySelector(
				`[data-lifecycle-track='${CSS.escape(symbol)}']`,
			) as HTMLElement | null,
			(lifecycleBySymbol[symbol] ?? "observing") as LifecycleState,
		);
	}

	const filteredEntries = (
		activeSymbol
			? entries.filter((entry) => {
					const symbol =
						Object.keys(entry.lifecycle ?? {})[0] ??
						Object.keys(entry.holdings ?? {})[0] ??
						"";
					return symbol === activeSymbol;
				})
			: entries
	).sort((left, right) => right.at.localeCompare(left.at));

	setText(
		root.querySelector("[data-journal='holdings-meta']"),
		`${activeSymbol ?? "all symbols"} · ${filteredEntries.length} entries`,
	);

	const holdingsEmpty = root.querySelector("[data-journal='holdings-empty']");

	if (holdingsEmpty instanceof HTMLElement) {
		holdingsEmpty.style.display = filteredEntries.length === 0 ? "" : "none";
		holdingsEmpty.textContent = online
			? "no journal entries retained"
			: "waiting for journal frames";
	}

	const entriesByKey = new Map(
		entries.map((entry) => [journalEntryKey(entry), entry]),
	);

	for (const key of journalKeys) {
		const row = root.querySelector(`[data-journal-entry='${CSS.escape(key)}']`);

		if (!(row instanceof HTMLElement)) {
			continue;
		}

		const entry = entriesByKey.get(key);

		if (entry === undefined) {
			row.style.display = "none";
			continue;
		}

		const symbol =
			Object.keys(entry.lifecycle ?? {})[0] ??
			Object.keys(entry.holdings ?? {})[0] ??
			"";
		const visible = !activeSymbol || symbol === activeSymbol;

		row.style.display = visible ? "" : "none";

		if (visible) {
			paintJournalEntryRow(row, entry);
		}
	}

	const activeFindings = activeSymbol
		? findings.filter((finding) => finding.symbol === activeSymbol)
		: findings;

	setText(
		root.querySelector("[data-journal='findings-meta']"),
		`${activeFindings.length} finding${activeFindings.length === 1 ? "" : "s"}`,
	);

	const findingsEmpty = root.querySelector("[data-journal='findings-empty']");

	if (findingsEmpty instanceof HTMLElement) {
		findingsEmpty.style.display = activeFindings.length === 0 ? "" : "none";
		findingsEmpty.textContent = online
			? "no postmortem findings"
			: "waiting for findings frames";
	}

	const findingsByKey = new Map(
		findings.map((finding) => [findingKey(finding), finding]),
	);

	for (const key of findingKeys) {
		const card = root.querySelector(`[data-finding='${CSS.escape(key)}']`);

		if (!(card instanceof HTMLElement)) {
			continue;
		}

		const finding = findingsByKey.get(key);

		if (finding === undefined) {
			card.style.display = "none";
			continue;
		}

		const visible = !activeSymbol || finding.symbol === activeSymbol;

		card.style.display = visible ? "" : "none";

		if (visible) {
			paintFindingCard(card, finding);
		}
	}
};
