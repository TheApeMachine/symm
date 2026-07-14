import { useSelector } from "@tanstack/react-store";
import { memo, useRef } from "react";
import {
	decisionStore,
	latestStrategyDecisions,
} from "#/collections/decisions";
import { type Execution, executionsStore } from "#/collections/executions";
import { positionsStore } from "#/collections/positions";
import { ColumnHeader } from "#/components/dashboard/header";
import { fixed } from "#/components/terminal/decision-format";
import { PositionGauge } from "#/components/terminal/position-gauge";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { cn } from "#/lib/utils";
import type { StrategyDecision } from "#/types/thesis";

const sameSymbols = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((symbol, index) => symbol === right[index]);

const decisionFraction = (decision: StrategyDecision): number =>
	decision.availableCapital > 0
		? decision.proposedNotional / decision.availableCapital
		: 0;

const actionBadgeClass = (action: string): string => {
	if (action === "exit") {
		return "bg-[color-mix(in_srgb,var(--down)_16%,transparent)] text-(--down)";
	}

	if (action === "enter") {
		return "bg-[color-mix(in_srgb,var(--up)_16%,transparent)] text-(--up)";
	}

	return "bg-[color-mix(in_srgb,var(--info)_16%,transparent)] text-(--info)";
};

type DecisionRowRefs = {
	symbol: HTMLDivElement | null;
	meta: HTMLDivElement | null;
	comb: HTMLSpanElement | null;
	fraction: HTMLSpanElement | null;
	action: HTMLSpanElement | null;
};

const paintDecisionRow = (refs: DecisionRowRefs, symbol: string): void => {
	const decision = decisionStore.state.decisions[symbol]?.values().at(-1);

	if (decision === undefined) {
		return;
	}

	if (refs.symbol !== null) {
		refs.symbol.textContent = decision.symbol;
	}

	if (refs.meta !== null) {
		refs.meta.textContent = `${decision.allocationClass} / ${decision.cause}`;
	}

	if (refs.comb !== null) {
		refs.comb.textContent = fixed(decision.utility);
	}

	if (refs.fraction !== null) {
		refs.fraction.textContent = `${(decisionFraction(decision) * 100).toFixed(2)}%`;
	}

	if (refs.action !== null) {
		refs.action.textContent = decision.action;
		refs.action.className = cn(
			"rounded-[2px] px-1.5 py-0.5 font-semibold text-[9px] uppercase",
			actionBadgeClass(decision.action),
		);
	}
};

/*
DecisionRow keeps one stable symbol shell mounted while direct paint refreshes the
latest circular decision without React reconciliation on every thesis tick.
*/
const DecisionRow = memo(({ symbol }: { symbol: string }) => {
	const symbolRef = useRef<HTMLDivElement>(null);
	const metaRef = useRef<HTMLDivElement>(null);
	const combRef = useRef<HTMLSpanElement>(null);
	const fractionRef = useRef<HTMLSpanElement>(null);
	const actionRef = useRef<HTMLSpanElement>(null);

	useDirectStorePaint(
		() =>
			paintDecisionRow(
				{
					symbol: symbolRef.current,
					meta: metaRef.current,
					comb: combRef.current,
					fraction: fractionRef.current,
					action: actionRef.current,
				},
				symbol,
			),
		[decisionStore],
		[symbol],
	);

	return (
		<div
			data-symbol={symbol}
			className="grid grid-cols-[78px_58px_minmax(84px,1fr)_72px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[11px]"
		>
			<div className="min-w-0">
				<div ref={symbolRef} className="truncate font-semibold text-(--f1)" />
				<div ref={metaRef} className="truncate text-[9px] text-(--f4)" />
			</div>
			<span ref={combRef} className="text-right text-(--f2)" />
			<span ref={fractionRef} className="truncate text-right text-(--f2)" />
			<span className="text-right">
				<span ref={actionRef} />
			</span>
		</div>
	);
});

const LiveDecisionEmpty = () => {
	const emptyRef = useRef<HTMLDivElement>(null);

	useDirectStorePaint(
		() => {
			if (emptyRef.current === null) {
				return;
			}

			emptyRef.current.style.display =
				Object.keys(decisionStore.state.decisions).length === 0 ? "" : "none";
		},
		[decisionStore],
		[],
	);

	return (
		<div ref={emptyRef} className="px-4 py-5 font-mono text-[11px] text-(--f4)">
			waiting for decision frames
		</div>
	);
};

const LiveDecisionRows = ({ symbols }: { symbols: string[] }) => (
	<div className="min-h-0 flex-1 overflow-auto">
		<div className="grid grid-cols-[78px_58px_minmax(84px,1fr)_72px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) uppercase">
			<span>Symbol</span>
			<span className="text-right">Comb</span>
			<span className="text-right">Fraction</span>
			<span className="text-right">Action</span>
		</div>
		<LiveDecisionEmpty />
		{symbols.map((symbol) => (
			<DecisionRow key={symbol} symbol={symbol} />
		))}
	</div>
);

type ExecutionAuditRow = {
	reason: string;
	reference: string;
	meta: string;
};

export const executionAuditRow = (execution: Execution): ExecutionAuditRow => {
	const reason = execution.order_status ?? execution.exec_type;

	if (typeof reason !== "string" || reason.length === 0) {
		throw new TypeError("execution requires order_status or exec_type");
	}

	const timestamp =
		typeof execution.timestamp === "string"
			? execution.timestamp.slice(11, 19)
			: "";
	const identifier =
		typeof execution.sequence === "number"
			? String(execution.sequence)
			: (execution.exec_id ?? execution.order_id ?? "");
	const trade =
		execution.last_qty !== undefined && execution.last_price !== undefined
			? `${execution.last_qty} @ ${fixed(Number(execution.last_price))}`
			: "";
	const meta = [
		execution.exec_type,
		execution.side,
		execution.symbol,
		trade,
	].filter((value) => typeof value === "string" && value.length > 0);

	return {
		reason,
		reference: [`#${identifier}`, timestamp].filter(Boolean).join(" · "),
		meta: meta.join(" · "),
	};
};

const PositionRows = ({
	symbols,
	quote,
	observed,
}: {
	symbols: string[];
	quote: string;
	observed: boolean;
}) => (
	<div className="min-h-0 flex-1 overflow-auto p-1.5">
		{symbols.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				{observed ? "no open positions" : "waiting for position frames"}
			</div>
		) : null}
		{symbols.slice(-8).map((symbol) => (
			<PositionGauge key={symbol} symbol={symbol} quote={quote} />
		))}
	</div>
);

const AuditRows = ({ executions }: { executions: Execution[] }) => (
	<div className="min-h-0 flex-1 overflow-auto py-0.5">
		{executions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				waiting for execution frames
			</div>
		) : null}
		{executions.map((execution) => {
			const row = executionAuditRow(execution);

			return (
				<div
					key={`${execution.exec_id ?? execution.order_id ?? "exec"}:${execution.order_status ?? ""}:${execution.timestamp ?? ""}:${execution.symbol ?? ""}:${execution.side ?? ""}`}
					className="border-(--line) border-b px-3 py-1.5"
				>
					<div className="flex items-start justify-between gap-2">
						<span className="font-medium text-[11px] text-(--f1)">
							{row.reason}
						</span>
						<span className="shrink-0 font-mono text-[9px] text-(--f4)">
							{row.reference}
						</span>
					</div>
					<div className="mt-px truncate font-mono text-[9px] text-(--f4)">
						{row.meta}
					</div>
				</div>
			);
		})}
	</div>
);

const LiveDecisionMeta = () => {
	const allowRef = useRef<HTMLSpanElement>(null);
	const denyRef = useRef<HTMLSpanElement>(null);

	useDirectStorePaint(
		() => {
			const decisions = latestStrategyDecisions(decisionStore.state.decisions);
			const active = decisions.filter(
				(decision) => decision.action === "enter" || decision.action === "exit",
			);
			const passive = decisions.filter(
				(decision) => decision.action !== "enter" && decision.action !== "exit",
			);

			if (allowRef.current !== null) {
				allowRef.current.textContent = String(active.length);
			}

			if (denyRef.current !== null) {
				denyRef.current.textContent = String(passive.length);
			}
		},
		[decisionStore],
		[],
	);

	return (
		<span>
			<span ref={allowRef} /> allow · <span ref={denyRef} /> deny
		</span>
	);
};

const LivePositionMeta = ({
	symbols,
	quote,
}: {
	symbols: string[];
	quote: string;
}) => {
	const prefixRef = useRef<HTMLSpanElement>(null);
	const countRef = useRef<HTMLSpanElement>(null);

	useDirectStorePaint(
		() => {
			const net = positionsStore.state.positions.reduce(
				(sum, position) => sum + position.pnl,
				0,
			);

			if (prefixRef.current !== null) {
				prefixRef.current.textContent =
					symbols.length === 0 ? "" : `net ${net.toFixed(4)} ${quote} · `;
				prefixRef.current.style.display = symbols.length === 0 ? "none" : "";
			}

			if (countRef.current !== null) {
				countRef.current.textContent = `${symbols.length} open`;
			}
		},
		[positionsStore],
		[symbols.length, quote],
	);

	return (
		<span>
			<span ref={prefixRef} />
			<span ref={countRef} />
		</span>
	);
};

export const DashboardRail = () => {
	const decisionSymbols = useSelector(
		decisionStore,
		(state) => Object.keys(state.decisions).sort(),
		{ compare: sameSymbols },
	);

	/*
	symbols only changes reference when a position opens or closes, not on
	every mark/pnl tick, so it is the one piece of positions state safe to
	drive React re-renders and row mount/unmount from. Each PositionGauge
	reads its own live values straight from the stores.
	*/
	const symbols = useSelector(
		positionsStore,
		(state) => state.positions.map((position) => position.symbol),
		{ compare: sameSymbols },
	);
	const observed = useSelector(positionsStore, (state) => state.observed);
	const quoteSymbol = useSelector(
		positionsStore,
		(state) => state.positions[0]?.symbol,
	);
	const quote = quoteSymbol?.split("/")[1] ?? "USD";
	const executions = useSelector(executionsStore, (state) => state.executions);

	return (
		<div className="flex min-h-0 flex-col bg-(--surface)">
			<div className="flex min-h-0 flex-[1.15] flex-col border-(--line) border-b">
				<ColumnHeader title="Decisions" meta={<LiveDecisionMeta />} />
				<LiveDecisionRows symbols={decisionSymbols} />
			</div>
			<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
				<ColumnHeader
					title="Open positions"
					meta={<LivePositionMeta symbols={symbols} quote={quote} />}
				/>
				<PositionRows symbols={symbols} quote={quote} observed={observed} />
			</div>
			<div className="flex min-h-0 flex-1 flex-col">
				<ColumnHeader title="Audit trail" />
				<AuditRows executions={[...executions].reverse()} />
			</div>
		</div>
	);
};
