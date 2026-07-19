import { useSelector } from "@tanstack/react-store";
import { useRef, useState } from "react";
import { appStore } from "#/collections/app";
import type { StrategyDecision } from "#/types/thesis";
import { fixed } from "#/components/terminal/decision-format";
import { TerminalSection } from "#/components/terminal/panels";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";
import { badgeVariants } from "@/components/ui/badge";
import { Panel } from "@/components/ui/panel";

const sameKeys = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((key, index) => key === right[index]);

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

/*
paintStrategyDecisions writes utility and attribution into mounted decision
shells so websocket cadence never re-renders StrategyDecisionRows.
*/
export const paintStrategyDecisions = (
	root: HTMLElement | null,
	decisions: StrategyDecision[],
	symbol: string | undefined,
): void => {
	if (root === null) {
		return;
	}

	const rows = symbol
		? decisions.filter((decision) => decision.symbol === symbol)
		: decisions;
	const keys = new Set(rows.map((decision) => decisionKey(decision)));

	setText(
		root.querySelector("[data-strategy='meta']"),
		`${rows.length} decision${rows.length === 1 ? "" : "s"}`,
	);

	const empty = root.querySelector("[data-strategy='empty']");

	if (empty instanceof HTMLElement) {
		empty.style.display = rows.length === 0 ? "" : "none";
	}

	for (const node of root.querySelectorAll("[data-strategy-row]")) {
		if (!(node instanceof HTMLElement)) {
			continue;
		}

		const key = node.getAttribute("data-strategy-row") ?? "";
		node.style.display = keys.has(key) ? "" : "none";
	}

	for (const decision of rows) {
		const row = root.querySelector(
			`[data-strategy-row="${CSS.escape(decisionKey(decision))}"]`,
		);

		if (!(row instanceof HTMLElement)) {
			continue;
		}

		setText(row.querySelector("[data-strategy='symbol']"), decision.symbol);

		const action = row.querySelector("[data-strategy='action']");

		if (action instanceof HTMLElement) {
			action.textContent = decision.action;
			action.className = badgeVariants({
				variant: actionVariant(decision.action),
				size: "xs",
			});
		}

		setText(
			row.querySelector("[data-strategy='utility']"),
			`utility ${fixed(decision.utility)} · ${decision.cause}`,
		);
		setText(row.querySelector("[data-strategy='reason']"), decision.reason);
		setText(
			row.querySelector("[data-strategy='notional']"),
			fixed(decision.proposedNotional),
		);
		setText(
			row.querySelector("[data-strategy='confidence']"),
			fixed(decision.confidence),
		);
		setText(
			row.querySelector("[data-strategy='expected']"),
			fixed(decision.expectedReturn),
		);
		setText(
			row.querySelector("[data-strategy='class']"),
			decision.allocationClass,
		);

		const alts = alternativeEntries(decision);
		const altNode = row.querySelector("[data-strategy='alts']");

		if (altNode instanceof HTMLElement) {
			altNode.style.display = alts === "" ? "none" : "";
			altNode.textContent = alts === "" ? "" : `alt · ${alts}`;
		}
	}
};

/*
StrategyDecisionRows mounts decision shells by identity and paints live utility
fields from the decisions store without React reconciliation each tick.
*/
export const StrategyDecisionRows = ({ symbol }: { symbol?: string }) => {
	const online = useSelector(appStore, (state) => state.online);
	const rootRef = useRef<HTMLDivElement>(null);
	const [keys, setKeys] = useState<string[]>([]);

	useDirectStorePaint(
		getWorker(),
		[{ store: "decisions", key: "" }],
		(buffers) => {
			const decisions = (buffers["decisions:"] ?? []) as StrategyDecision[];
			const scoped = symbol
				? decisions.filter((decision) => decision.symbol === symbol)
				: decisions;
			const nextKeys = [
				...new Set(scoped.map((decision) => decisionKey(decision))),
			].sort();

			setKeys((previous) =>
				sameKeys(previous, nextKeys) ? previous : nextKeys,
			);
			paintStrategyDecisions(rootRef.current, decisions, symbol);
		},
		[online, symbol],
	);

	return (
		<div ref={rootRef}>
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
					<div className="flex flex-col gap-2">
						{keys.map((key) => (
							<Panel
								key={key}
								variant="surface"
								size="bare"
								data-strategy-row={key}
								className="px-3 py-2.5"
								style={{ display: "none" }}
							>
								<div className="flex items-center justify-between gap-2">
									<div
										data-strategy="symbol"
										className="font-mono font-semibold text-[12px] text-(--f1)"
									/>
									<span data-strategy="action" />
								</div>
								<div
									data-strategy="utility"
									className="mt-1 font-mono text-[10px] text-(--f3)"
								/>
								<div
									data-strategy="reason"
									className="mt-1 font-mono text-[9.5px] text-(--f4)"
								/>
								<div className="mt-2 grid grid-cols-2 gap-1.5 font-mono text-[9.5px]">
									<div className="text-(--f4)">notional</div>
									<div
										data-strategy="notional"
										className="text-right text-(--f2)"
									/>
									<div className="text-(--f4)">confidence</div>
									<div
										data-strategy="confidence"
										className="text-right text-(--f2)"
									/>
									<div className="text-(--f4)">expected return</div>
									<div
										data-strategy="expected"
										className="text-right text-(--f2)"
									/>
									<div className="text-(--f4)">class</div>
									<div
										data-strategy="class"
										className="text-right text-(--f2)"
									/>
								</div>
								<div
									data-strategy="alts"
									className="mt-2 border-(--line) border-t pt-2 font-mono text-[9px] text-(--f4)"
									style={{ display: "none" }}
								/>
							</Panel>
						))}
					</div>
				</div>
			</TerminalSection>
		</div>
	);
};
