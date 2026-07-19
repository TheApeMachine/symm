import { useSelector } from "@tanstack/react-store";
import { memo, useRef, useState } from "react";
import { appStore } from "#/collections/app";
import type { Holding, LifecycleRow, StrategyDecision } from "#/collections/types";
import { ColumnHeader } from "#/components/dashboard/header";
import { fixed } from "#/components/terminal/decision-format";
import { PositionGauge } from "#/components/terminal/position-gauge";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { cn } from "#/lib/utils";
import { getWorker } from "#/providers/websocket";

const sameSymbols = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((symbol, index) => symbol === right[index]);

const isOpenLot = (holding: Holding): boolean =>
	holding.qty > 0 &&
	holding.status !== "closed" &&
	holding.status !== "canceled";

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

type DecisionRowRefs = {
	symbol: HTMLDivElement | null;
	meta: HTMLDivElement | null;
	comb: HTMLSpanElement | null;
	fraction: HTMLSpanElement | null;
	action: HTMLSpanElement | null;
};

const paintDecisionRow = (
	refs: DecisionRowRefs,
	decision: StrategyDecision | undefined,
): void => {
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
		const fraction = decisionFraction(decision);
		refs.fraction.textContent =
			fraction === null ? "—" : `${(fraction * 100).toFixed(2)}%`;
	}

	if (refs.action !== null) {
		refs.action.textContent = decision.action;
		refs.action.className = cn(
			"rounded-[2px] px-1.5 py-0.5 font-semibold text-[9px] uppercase",
			actionBadgeClass(decision.action),
		);
	}
};

const DecisionRow = memo(({ symbol }: { symbol: string }) => {
	const symbolRef = useRef<HTMLDivElement>(null);
	const metaRef = useRef<HTMLDivElement>(null);
	const combRef = useRef<HTMLSpanElement>(null);
	const fractionRef = useRef<HTMLSpanElement>(null);
	const actionRef = useRef<HTMLSpanElement>(null);
	const online = useSelector(appStore, (state) => state.online);

	useDirectStorePaint(
		getWorker(),
		[{ store: "decisions", key: symbol }],
		(buffers) =>
			paintDecisionRow(
				{
					symbol: symbolRef.current,
					meta: metaRef.current,
					comb: combRef.current,
					fraction: fractionRef.current,
					action: actionRef.current,
				},
				(buffers[`decisions:${symbol}`] ?? []).at(-1) as
					| StrategyDecision
					| undefined,
			),
		[online, symbol],
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

const LiveDecisionEmpty = ({ empty, observed }: { empty: boolean; observed: boolean }) => (
	<div
		className="px-4 py-5 font-mono text-[11px] text-(--f4)"
		style={{ display: empty ? "" : "none" }}
	>
		{observed ? "no current decisions" : "waiting for decision frames"}
	</div>
);

const LiveDecisionRows = ({
	symbols,
	empty,
	observed,
}: {
	symbols: string[];
	empty: boolean;
	observed: boolean;
}) => (
	<div className="min-h-0 flex-1 overflow-auto">
		<div className="grid grid-cols-[78px_58px_minmax(84px,1fr)_72px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) uppercase">
			<span>Symbol</span>
			<span className="text-right">Comb</span>
			<span className="text-right">Fraction</span>
			<span className="text-right">Action</span>
		</div>
		<LiveDecisionEmpty empty={empty} observed={observed} />
		{symbols.map((symbol) => (
			<DecisionRow key={symbol} symbol={symbol} />
		))}
	</div>
);

type AuditRow = {
	reason: string;
	reference: string;
	meta: string;
};

export const isClosedLot = (position: Holding): boolean => {
	const status = position.status;

	return typeof status === "string" && status === "closed";
};

/*
holdingAuditRow formats one retained holding for the dashboard audit rail.
*/
export const holdingAuditRow = (
	position: Holding,
	lifecycle?: string,
): AuditRow => {
	const phase =
		lifecycle ??
		(typeof position.status === "string" ? position.status : "closed");
	const pnl = Number.isFinite(position.pnl) ? fixed(position.pnl) : "—";
	const ret = Number.isFinite(position.return_pct)
		? `${(position.return_pct * 100).toFixed(2)}%`
		: "—";

	return {
		reason: phase,
		reference: position.symbol,
		meta: `pnl ${pnl} · return ${ret}`,
	};
};

export const auditHoldings = (holdings: Holding[]): Holding[] =>
	holdings.filter(isClosedLot);

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
				{observed ? "no open holdings" : "waiting for holdings frames"}
			</div>
		) : null}
		{symbols.slice(-8).map((symbol) => (
			<PositionGauge key={symbol} symbol={symbol} quote={quote} />
		))}
	</div>
);

const AuditRows = ({
	holdings,
	lifecycle,
	observed,
}: {
	holdings: Holding[];
	lifecycle: Record<string, string>;
	observed: boolean;
}) => (
	<div className="min-h-0 flex-1 overflow-auto py-0.5">
		{holdings.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				{observed ? "no closed lots yet" : "waiting for position frames"}
			</div>
		) : null}
		{holdings.map((holding) => {
			const row = holdingAuditRow(holding, lifecycle[holding.symbol]);

			return (
				<div
					key={`${holding.symbol}:${String(holding.status)}`}
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

const LiveDecisionMeta = ({
	active,
	passive,
}: {
	active: number;
	passive: number;
}) => (
	<span>
		{active} active · {passive} passive
	</span>
);

const LivePositionMeta = ({
	symbols,
	quote,
	net,
}: {
	symbols: string[];
	quote: string;
	net: number;
}) => (
	<span>
		{symbols.length === 0 ? null : `net ${net.toFixed(4)} ${quote} · `}
		{symbols.length} open
	</span>
);

export const DashboardRail = () => {
	const online = useSelector(appStore, (state) => state.online);
	const [decisionSymbols, setDecisionSymbols] = useState<string[]>([]);
	const [decisionObserved, setDecisionObserved] = useState(false);
	const [decisionActive, setDecisionActive] = useState(0);
	const [decisionPassive, setDecisionPassive] = useState(0);
	const [symbols, setSymbols] = useState<string[]>([]);
	const [observed, setObserved] = useState(false);
	const [quote, setQuote] = useState("USD");
	const [net, setNet] = useState(0);
	const [closedLots, setClosedLots] = useState<Holding[]>([]);
	const [lifecycle, setLifecycle] = useState<Record<string, string>>({});

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "decisions", key: "" },
			{ store: "holdings", key: "" },
			{ store: "lifecycle", key: "" },
		],
		(buffers) => {
			const decisions = (buffers["decisions:"] ?? []) as StrategyDecision[];
			const holdings = (buffers["holdings:"] ?? []) as Holding[];
			const lifecycleRows = (buffers["lifecycle:"] ?? []) as LifecycleRow[];
			const nextDecisionSymbols = [
				...new Set(decisions.map((decision) => decision.symbol)),
			].sort();
			const active = decisions.filter(
				(decision) => decision.action === "enter" || decision.action === "exit",
			);
			const passive = decisions.filter(
				(decision) => decision.action !== "enter" && decision.action !== "exit",
			);
			const open = holdings.filter(isOpenLot);
			const nextSymbols = open.map((holding) => holding.symbol);
			const nextNet = open.reduce((sum, holding) => sum + (holding.pnl ?? 0), 0);
			const nextQuote = open[0]?.symbol.split("/")[1] ?? "USD";
			const nextLifecycle = Object.fromEntries(
				lifecycleRows.map((row) => [row.symbol, String(row.state)]),
			);

			setDecisionObserved(true);
			setDecisionActive(active.length);
			setDecisionPassive(passive.length);
			setDecisionSymbols((prev) =>
				sameSymbols(prev, nextDecisionSymbols) ? prev : nextDecisionSymbols,
			);
			setObserved(true);
			setSymbols((prev) => (sameSymbols(prev, nextSymbols) ? prev : nextSymbols));
			setQuote(nextQuote);
			setNet(nextNet);
			setClosedLots(auditHoldings(holdings));
			setLifecycle(nextLifecycle);
		},
		[online],
	);

	return (
		<div className="flex min-h-0 flex-col bg-(--surface)">
			<div className="flex min-h-0 flex-[1.15] flex-col border-(--line) border-b">
				<ColumnHeader
					title="Decisions"
					meta={
						<LiveDecisionMeta
							active={decisionActive}
							passive={decisionPassive}
						/>
					}
				/>
				<LiveDecisionRows
					symbols={decisionSymbols}
					empty={decisionSymbols.length === 0}
					observed={decisionObserved}
				/>
			</div>
			<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
				<ColumnHeader
					title="Open positions"
					meta={
						<LivePositionMeta symbols={symbols} quote={quote} net={net} />
					}
				/>
				<PositionRows symbols={symbols} quote={quote} observed={observed} />
			</div>
			<div className="flex min-h-0 flex-1 flex-col">
				<ColumnHeader title="Audit trail" />
				<AuditRows
					holdings={[...closedLots].reverse()}
					lifecycle={lifecycle}
					observed={observed}
				/>
			</div>
		</div>
	);
};
