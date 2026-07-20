import type { StrategyDecision } from "#/types/thesis";
import { fixed } from "#/components/terminal/decision-format";
import { cn } from "#/lib/utils";

/*
decisionFraction is the Allocator-sized notional share of available capital.
*/
export const decisionFraction = (decision: StrategyDecision): number | null => {
	const notional = Number(decision.proposedNotional);
	const capital = Number(decision.availableCapital);

	if (!(notional > 0) || !(capital > 0) || !Number.isFinite(capital)) {
		return null;
	}

	return notional / capital;
};

const actionBadgeClass = (action: string): string => {
	if (action === "exit") {
		return "bg-[color-mix(in_srgb,var(--down)_16%,transparent)] text-(--down)";
	}

	if (action === "enter") {
		return "bg-[color-mix(in_srgb,var(--up)_16%,transparent)] text-(--up)";
	}

	return "bg-[color-mix(in_srgb,var(--info)_16%,transparent)] text-(--info)";
};

type DecisionRowParts = {
	symbolEl: HTMLDivElement | null;
	meta: HTMLDivElement | null;
	comb: HTMLSpanElement | null;
	fraction: HTMLSpanElement | null;
	action: HTMLSpanElement | null;
};

export const decisionRows = new Map<string, DecisionRowParts>();

const decisionRowClassName =
	"grid grid-cols-[78px_58px_minmax(84px,1fr)_72px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[11px]";

const bindDecisionRowParts = (
	symbol: string,
	root: HTMLElement | null,
): void => {
	if (root === null) {
		decisionRows.delete(symbol);
		return;
	}

	decisionRows.set(symbol, {
		symbolEl: root.querySelector('[data-decision-row="symbol"]'),
		meta: root.querySelector('[data-decision-row="meta"]'),
		comb: root.querySelector('[data-decision-row="comb"]'),
		fraction: root.querySelector('[data-decision-row="fraction"]'),
		action: root.querySelector('[data-decision-row="action"]'),
	});
};

/*
createDecisionRowElement builds one dashboard decision shell and binds paint
targets into decisionRows for DRAW updates.
*/
export const createDecisionRowElement = (symbol: string): HTMLElement => {
	const root = document.createElement("div");
	root.dataset.symbol = symbol;
	root.className = decisionRowClassName;

	const symbolWrap = document.createElement("div");
	symbolWrap.className = "min-w-0";

	const symbolEl = document.createElement("div");
	symbolEl.dataset.decisionRow = "symbol";
	symbolEl.className = "truncate font-semibold text-(--f1)";

	const meta = document.createElement("div");
	meta.dataset.decisionRow = "meta";
	meta.className = "truncate text-[9px] text-(--f4)";

	symbolWrap.append(symbolEl, meta);

	const comb = document.createElement("span");
	comb.dataset.decisionRow = "comb";
	comb.className = "text-right text-(--f2)";

	const fraction = document.createElement("span");
	fraction.dataset.decisionRow = "fraction";
	fraction.className = "truncate text-right text-(--f2)";

	const actionWrap = document.createElement("span");
	actionWrap.className = "text-right";

	const action = document.createElement("span");
	action.dataset.decisionRow = "action";
	actionWrap.append(action);

	root.append(symbolWrap, comb, fraction, actionWrap);
	bindDecisionRowParts(symbol, root);

	return root;
};

/*
removeDecisionRow drops one symbol from the decisionRows paint registry.
*/
export const removeDecisionRow = (symbol: string): void => {
	decisionRows.delete(symbol);
};

const writeDecisionRow = (
	parts: DecisionRowParts,
	decision: StrategyDecision | undefined,
): void => {
	if (decision === undefined) {
		return;
	}

	if (parts.symbolEl !== null) {
		parts.symbolEl.textContent = decision.symbol;
	}

	if (parts.meta !== null) {
		parts.meta.textContent = `${decision.allocationClass} / ${decision.cause}`;
	}

	if (parts.comb !== null) {
		parts.comb.textContent = fixed(decision.utility);
	}

	if (parts.fraction !== null) {
		const fraction = decisionFraction(decision);
		parts.fraction.textContent =
			fraction === null ? "—" : `${(fraction * 100).toFixed(2)}%`;
	}

	if (parts.action !== null) {
		parts.action.textContent = decision.action;
		parts.action.className = cn(
			"rounded-[2px] px-1.5 py-0.5 font-semibold text-[9px] uppercase",
			actionBadgeClass(decision.action),
		);
	}
};

/*
paintDecisionSymbol paints one DRAW decision into its bound DecisionRow shell.
*/
export const paintDecisionSymbol = (decision: StrategyDecision) => {
	const parts = decisionRows.get(decision.symbol);

	if (parts !== undefined) {
		writeDecisionRow(parts, decision);
	}
};
