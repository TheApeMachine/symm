import { createRef, useEffect } from "react";
import type { StrategyDecision } from "#/types/thesis";
import { readDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { fixed } from "#/components/terminal/decision-format";
import { TerminalSection } from "#/components/terminal/terminal-section";
import { badgeVariants } from "@/components/ui/badge";
import { Panel } from "@/components/ui/panel";
import { registerPainter } from "#/providers/ws-stores";

const strategyRootRef = createRef<HTMLDivElement>();
const strategyListRef = createRef<HTMLDivElement>();

type StrategyRowParts = {
	card: HTMLElement;
	symbol: HTMLElement;
	action: HTMLElement;
	utility: HTMLElement;
	reason: HTMLElement;
	notional: HTMLElement;
	confidence: HTMLElement;
	expected: HTMLElement;
	classEl: HTMLElement;
	alts: HTMLElement;
};

const strategyRows = new Map<string, StrategyRowParts>();

const decisionKey = (decision: StrategyDecision): string =>
	`${decision.symbol}:${decision.action}:${decision.at}`;

const alternativeEntries = (decision: StrategyDecision): string =>
	Object.entries(decision.alternatives ?? {})
		.sort((left, right) => right[1] - left[1])
		.slice(0, 4)
		.map(([action, utility]) => `${action} ${utility.toFixed(3)}`)
		.join(" · ");

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

const actionVariant = (
	action: string,
): "success" | "warning" | "info" => {
	if (action === "enter") {
		return "success";
	}

	if (action === "exit") {
		return "warning";
	}

	return "info";
};

const bindStrategyRow = (key: string): StrategyRowParts => {
	const card = document.createElement("div");
	card.dataset.strategyRow = key;
	card.className =
		"border border-(--line) bg-(--surface) rounded-[3px] px-3 py-2.5";

	const head = document.createElement("div");
	head.className = "flex items-center justify-between gap-2";

	const symbol = document.createElement("div");
	symbol.dataset.strategy = "symbol";
	symbol.className = "font-mono font-semibold text-[12px] text-(--f1)";

	const action = document.createElement("span");
	action.dataset.strategy = "action";
	head.append(symbol, action);

	const utility = document.createElement("div");
	utility.dataset.strategy = "utility";
	utility.className = "mt-1 font-mono text-[10px] text-(--f3)";

	const reason = document.createElement("div");
	reason.dataset.strategy = "reason";
	reason.className = "mt-1 font-mono text-[9.5px] text-(--f4)";

	const grid = document.createElement("div");
	grid.className =
		"mt-2 grid grid-cols-2 gap-1.5 font-mono text-[9.5px]";

	const notionalLabel = document.createElement("div");
	notionalLabel.className = "text-(--f4)";
	notionalLabel.textContent = "notional";

	const notional = document.createElement("div");
	notional.dataset.strategy = "notional";
	notional.className = "text-right text-(--f2)";

	const confidenceLabel = document.createElement("div");
	confidenceLabel.className = "text-(--f4)";
	confidenceLabel.textContent = "confidence";

	const confidence = document.createElement("div");
	confidence.dataset.strategy = "confidence";
	confidence.className = "text-right text-(--f2)";

	const expectedLabel = document.createElement("div");
	expectedLabel.className = "text-(--f4)";
	expectedLabel.textContent = "expected return";

	const expected = document.createElement("div");
	expected.dataset.strategy = "expected";
	expected.className = "text-right text-(--f2)";

	const classLabel = document.createElement("div");
	classLabel.className = "text-(--f4)";
	classLabel.textContent = "class";

	const classEl = document.createElement("div");
	classEl.dataset.strategy = "class";
	classEl.className = "text-right text-(--f2)";

	grid.append(
		notionalLabel,
		notional,
		confidenceLabel,
		confidence,
		expectedLabel,
		expected,
		classLabel,
		classEl,
	);

	const alts = document.createElement("div");
	alts.dataset.strategy = "alts";
	alts.className =
		"mt-2 border-(--line) border-t pt-2 font-mono text-[9px] text-(--f4)";
	alts.style.display = "none";

	card.append(head, utility, reason, grid, alts);

	return {
		card,
		symbol,
		action,
		utility,
		reason,
		notional,
		confidence,
		expected,
		classEl,
		alts,
	};
};

const paintStrategyRow = (
	parts: StrategyRowParts,
	decision: StrategyDecision,
): void => {
	setText(parts.symbol, decision.symbol);

	parts.action.textContent = decision.action;
	parts.action.className = badgeVariants({
		variant: actionVariant(decision.action),
		size: "xs",
	});

	setText(
		parts.utility,
		`utility ${fixed(decision.utility)} · ${decision.cause}`,
	);
	setText(parts.reason, decision.reason);
	setText(parts.notional, fixed(decision.proposedNotional));
	setText(parts.confidence, fixed(decision.confidence));
	setText(parts.expected, fixed(decision.expectedReturn));
	setText(parts.classEl, decision.allocationClass);

	const alts = alternativeEntries(decision);
	parts.alts.style.display = alts === "" ? "none" : "";
	parts.alts.textContent = alts === "" ? "" : `alt · ${alts}`;
};

const writeStrategyDecisions = (
	decisions: StrategyDecision[],
	symbol: string | undefined,
): void => {
	const root = strategyRootRef.current;
	const list = strategyListRef.current;

	if (root === null || list === null) {
		return;
	}

	const rows = symbol
		? decisions.filter((decision) => decision.symbol === symbol)
		: decisions;
	const nextKeys = new Set(rows.map((decision) => decisionKey(decision)));
	const ordered: HTMLElement[] = [];

	setText(
		root.querySelector("[data-strategy='meta']"),
		`${rows.length} decision${rows.length === 1 ? "" : "s"}`,
	);

	const empty = root.querySelector("[data-strategy='empty']");

	if (empty instanceof HTMLElement) {
		empty.style.display = rows.length === 0 ? "" : "none";
	}

	for (const decision of rows) {
		const key = decisionKey(decision);
		let parts = strategyRows.get(key);

		if (parts === undefined) {
			parts = bindStrategyRow(key);
			strategyRows.set(key, parts);
		}

		paintStrategyRow(parts, decision);
		ordered.push(parts.card);
	}

	for (const [key, parts] of strategyRows) {
		if (nextKeys.has(key)) {
			continue;
		}

		parts.card.remove();
		strategyRows.delete(key);
	}

	const orderMatches =
		ordered.length === list.children.length &&
		ordered.every((card, index) => list.children[index] === card);

	if (!orderMatches) {
		list.replaceChildren(...ordered);
	}
};

/*
paintStrategyDecisions paints the current DRAW decisions batch into the static
StrategyDecisionRows shell, creating row shells only when identities change.
*/
export const paintStrategyDecisions = (value: unknown) => {
	const decisions = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as StrategyDecision[];
	const scope = readDecisionsScopeSymbol();
	const symbol =
		scope !== undefined && scope !== ""
			? scope
			: undefined;

	writeStrategyDecisions(decisions, symbol);
};

/*
StrategyDecisionRows is the static strategy-intent shell. DRAW paints live fields
via paintStrategyDecisions without React reconciliation each tick.
*/
export const StrategyDecisionRows = () => {
	useEffect(() => registerPainter("decisions", paintStrategyDecisions), []);

	return (
		<div ref={strategyRootRef}>
			<TerminalSection
				title="Strategy intent"
				meta={<span data-strategy="meta">0 decisions</span>}
				className="mt-4"
			>
				<div className="min-h-0 flex-1 overflow-auto p-2">
					<Panel
						variant="surface"
						size="bare"
						data-strategy="empty"
						className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)"
					>
						waiting for strategy decision frames
					</Panel>
					<div
						ref={strategyListRef}
						className="flex flex-col gap-2"
					/>
				</div>
			</TerminalSection>
		</div>
	);
};
