import type { Holding } from "#/collections/types";
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

const isOpenLot = (holding: Holding): boolean =>
	Number(holding.qty) > 0 &&
	holding.status !== "closed" &&
	holding.status !== "canceled";

export const holdingKey = (holding: Holding): string =>
	`${holding.symbol}:${String(holding.status)}:${holding.qty}`;

export const findingKey = (finding: Finding): string =>
	`${finding.symbol}:${finding.component}:${finding.condition}`;

const paintHoldingRow = (row: HTMLElement, holding: Holding): void => {
	setText(row.querySelector("[data-journal='holding-symbol']"), holding.symbol);
	setText(
		row.querySelector("[data-journal='holding-meta']"),
		[
			`qty ${fixed(holding.qty)}`,
			`bid ${fixed(holding.mark)}`,
			`pnl ${fixed(holding.pnl)}`,
			isOpenLot(holding) ? "" : "closed",
		]
			.filter(Boolean)
			.join(" · "),
	);

	const status = row.querySelector("[data-journal='holding-status']");

	if (!(status instanceof HTMLElement)) {
		return;
	}

	status.textContent =
		typeof holding.status === "string" ? holding.status : "unknown";
	status.className = cn(
		badgeVariants({
			variant: isOpenLot(holding) ? "success" : "info",
			size: "xs",
		}),
	);
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
paintJournalSurface writes lifecycle rails, holdings lots, and postmortem
findings into mounted shells so websocket cadence never re-renders JournalSurface.
*/
export const paintJournalSurface = (
	root: HTMLElement | null,
	input: {
		activeSymbol: string | null;
		findings: Finding[];
		findingKeys: string[];
		holdings: Holding[];
		holdingKeys: string[];
		lifecycleBySymbol: Record<string, string>;
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
		findings,
		findingKeys,
		holdings,
		holdingKeys,
		lifecycleBySymbol,
		onLifecycleSelect,
		online,
		symbols,
	} = input;

	syncJournalShells(root, {
		symbols,
		onLifecycleSelect,
		holdingKeys,
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

	const filteredHoldings = (
		activeSymbol
			? holdings.filter((holding) => holding.symbol === activeSymbol)
			: holdings
	).sort((left, right) => left.symbol.localeCompare(right.symbol));

	setText(
		root.querySelector("[data-journal='holdings-meta']"),
		`${activeSymbol ?? "all symbols"} · ${filteredHoldings.length} lots`,
	);

	const holdingsEmpty = root.querySelector("[data-journal='holdings-empty']");

	if (holdingsEmpty instanceof HTMLElement) {
		holdingsEmpty.style.display = filteredHoldings.length === 0 ? "" : "none";
		holdingsEmpty.textContent = online
			? "no holdings retained"
			: "waiting for position frames";
	}

	const holdingsByKey = new Map(
		holdings.map((holding) => [holdingKey(holding), holding]),
	);

	for (const key of holdingKeys) {
		const row = root.querySelector(`[data-holding='${CSS.escape(key)}']`);

		if (!(row instanceof HTMLElement)) {
			continue;
		}

		const holding = holdingsByKey.get(key);

		if (holding === undefined) {
			row.style.display = "none";
			continue;
		}

		const visible = !activeSymbol || holding.symbol === activeSymbol;

		row.style.display = visible ? "" : "none";

		if (visible) {
			paintHoldingRow(row, holding);
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
