import { createRef } from "react";
import type { Holding, LifecycleRow } from "#/collections/types";
import type { StrategyDecision } from "#/types/thesis";
import { ColumnHeader } from "#/components/dashboard/header";
import { DashboardAuditSync } from "#/components/terminal/dashboard-audit";
import { DashboardShellSync } from "#/components/terminal/dashboard-shells";
import { paintDecisionSymbol } from "#/components/terminal/decision-row";

export {
	auditHoldings,
	holdingAuditRow,
	isClosedLot,
} from "#/components/terminal/dashboard-audit";
export { decisionFraction } from "#/components/terminal/decision-row";

const isOpenLot = (holding: Holding): boolean =>
	holding.qty > 0 &&
	holding.status !== "closed" &&
	holding.status !== "canceled";

const decisionActiveRef = createRef<HTMLSpanElement>();
const decisionPassiveRef = createRef<HTMLSpanElement>();
const positionMetaRef = createRef<HTMLSpanElement>();
const decisionEmptyRef = createRef<HTMLDivElement>();
const positionEmptyRef = createRef<HTMLDivElement>();
const auditEmptyRef = createRef<HTMLDivElement>();
const auditListRef = createRef<HTMLDivElement>();
const decisionListRef = createRef<HTMLDivElement>();
const positionListRef = createRef<HTMLDivElement>();

const shellSync = new DashboardShellSync();
const auditSync = new DashboardAuditSync();

let lastHoldings: Holding[] = [];
let lastLifecycle: Record<string, string> = {};
let lastDecisionSymbols: string[] = [];

const setText = (element: HTMLElement | null, value: string) => {
	if (element !== null) {
		element.textContent = value;
	}
};

/*
paintDecisionRows paints the DRAW decisions batch into bound decision shells
and refreshes the dashboard decisions meta strip.
*/
export const paintDecisionRows = (value: unknown, _focusSymbol: string) => {
	const decisions = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as StrategyDecision[];
	const nextSymbols = [
		...new Set(decisions.map((decision) => decision.symbol)),
	].sort();
	const active = decisions.filter(
		(decision) => decision.action === "enter" || decision.action === "exit",
	);
	const passive = decisions.filter(
		(decision) => decision.action !== "enter" && decision.action !== "exit",
	);

	if (
		nextSymbols.length !== lastDecisionSymbols.length ||
		nextSymbols.some(
			(symbol, index) => symbol !== lastDecisionSymbols[index],
		)
	) {
		lastDecisionSymbols = nextSymbols;
		shellSync.syncDecisionShells(nextSymbols, decisionListRef.current);
	}

	setText(decisionActiveRef.current, String(active.length));
	setText(decisionPassiveRef.current, String(passive.length));

	if (decisionEmptyRef.current !== null) {
		decisionEmptyRef.current.style.display =
			nextSymbols.length === 0 ? "" : "none";
		decisionEmptyRef.current.textContent =
			decisions.length === 0
				? "waiting for decision frames"
				: "no current decisions";
	}

	for (const decision of decisions) {
		paintDecisionSymbol(decision);
	}
};

const writePositionMeta = (open: Holding[]) => {
	const symbols = open.map((holding) => holding.symbol);
	const net = open.reduce((sum, holding) => sum + (holding.pnl ?? 0), 0);
	const quote = shellSync.refreshQuote(open);

	shellSync.syncPositionShells(symbols, quote, positionListRef.current);

	if (positionEmptyRef.current !== null) {
		positionEmptyRef.current.style.display =
			symbols.length === 0 ? "" : "none";
		positionEmptyRef.current.textContent =
			lastHoldings.length === 0
				? "waiting for holdings frames"
				: "no open holdings";
	}

	if (positionMetaRef.current !== null) {
		positionMetaRef.current.textContent =
			symbols.length === 0
				? `${symbols.length} open`
				: `net ${net.toFixed(4)} ${quote} · ${symbols.length} open`;
	}
};

/*
paintDashboardHoldings refreshes open-position shells and the audit trail from
the current DRAW holdings batch.
*/
export const paintDashboardHoldings = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastHoldings = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Holding[];
	writePositionMeta(lastHoldings.filter(isOpenLot));
	auditSync.writeAudit(
		lastHoldings,
		lastLifecycle,
		auditListRef.current,
		auditEmptyRef.current,
	);
};

/*
paintDashboardLifecycle refreshes audit lifecycle labels from the current DRAW
lifecycle batch.
*/
export const paintDashboardLifecycle = (
	value: unknown,
	_focusSymbol: string,
) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as LifecycleRow[];
	lastLifecycle = Object.fromEntries(
		rows.map((row) => [row.symbol, String(row.state)]),
	);
	auditSync.writeAudit(
		lastHoldings,
		lastLifecycle,
		auditListRef.current,
		auditEmptyRef.current,
	);
};

/*
DashboardRail is the static decisions / open-lots / audit shell. DRAW paints via
paintDecisionRows, paintDashboardHoldings, and paintDashboardLifecycle.
*/
export const DashboardRail = () => (
	<div className="flex min-h-0 flex-col bg-(--surface)">
		<div className="flex min-h-0 flex-[1.15] flex-col border-(--line) border-b">
			<ColumnHeader
				title="Decisions"
				meta={
					<span>
						<span ref={decisionActiveRef}>0</span> active ·{" "}
						<span ref={decisionPassiveRef}>0</span> passive
					</span>
				}
			/>
			<div className="min-h-0 flex-1 overflow-auto">
				<div className="grid grid-cols-[78px_58px_minmax(84px,1fr)_72px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) uppercase">
					<span>Symbol</span>
					<span className="text-right">Comb</span>
					<span className="text-right">Fraction</span>
					<span className="text-right">Action</span>
				</div>
				<div
					ref={decisionEmptyRef}
					className="px-4 py-5 font-mono text-[11px] text-(--f4)"
				>
					waiting for decision frames
				</div>
				<div ref={decisionListRef} />
			</div>
		</div>
		<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
			<ColumnHeader
				title="Open positions"
				meta={<span ref={positionMetaRef}>0 open</span>}
			/>
			<div className="min-h-0 flex-1 overflow-auto p-1.5">
				<div
					ref={positionEmptyRef}
					className="px-4 py-5 font-mono text-[11px] text-(--f4)"
				>
					waiting for holdings frames
				</div>
				<div ref={positionListRef} />
			</div>
		</div>
		<div className="flex min-h-0 flex-1 flex-col">
			<ColumnHeader title="Audit trail" />
			<div className="min-h-0 flex-1 overflow-auto py-0.5">
				<div
					ref={auditEmptyRef}
					className="px-4 py-5 font-mono text-[11px] text-(--f4)"
				>
					waiting for position frames
				</div>
				<div ref={auditListRef} />
			</div>
		</div>
	</div>
);
