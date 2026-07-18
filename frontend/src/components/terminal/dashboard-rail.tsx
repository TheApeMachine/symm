import { useSelector } from "@tanstack/react-store";
import { memo, useRef } from "react";
import {
	decisionStore,
	latestStrategyDecisions,
} from "#/collections/decisions";
import { positionsStore } from "#/collections/positions";
import {
	tradeJournalStore,
	tradeJournalValues,
	tradeObservationKey,
} from "#/collections/trade-journal";
import { ColumnHeader } from "#/components/dashboard/header";
import { fixed } from "#/components/terminal/decision-format";
import { PositionGauge } from "#/components/terminal/position-gauge";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { requirePositive } from "#/lib/domain";
import { cn } from "#/lib/utils";
import type { StrategyDecision, TradeObservation } from "#/types/thesis";

const sameSymbols = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((symbol, index) => symbol === right[index]);

/*
decisionFraction is the deployable notional share of available capital.
Undefined when capital is not strictly positive.
*/
const decisionFraction = (decision: StrategyDecision): number => {
	const capital = requirePositive(
		decision.availableCapital,
		"decision.availableCapital",
	);

	return decision.proposedNotional / capital;
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
		try {
			refs.fraction.textContent = `${(decisionFraction(decision) * 100).toFixed(2)}%`;
		} catch {
			refs.fraction.textContent = "—";
		}
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

			const empty = Object.keys(decisionStore.state.decisions).length === 0;
			emptyRef.current.style.display = empty ? "" : "none";
			emptyRef.current.textContent = decisionStore.state.observed
				? "no current decisions"
				: "waiting for decision frames";
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

type AuditRow = {
	reason: string;
	reference: string;
	meta: string;
};

const auditEntryLifecycle = new Set(["partially_entered", "entered"]);

const auditExitLifecycle = new Set(["partially_exited", "closed"]);

const isEntryObservation = (observation: TradeObservation): boolean => {
	if (
		(observation.kind === "execution_error" ||
			observation.kind === "broker_rejection") &&
		observation.action === "enter"
	) {
		return true;
	}

	if (observation.kind === "execution" && observation.side === "buy") {
		return true;
	}

	if (
		observation.kind === "lifecycle_transition" &&
		typeof observation.status === "string"
	) {
		return auditEntryLifecycle.has(observation.status);
	}

	return false;
};

const isExitObservation = (observation: TradeObservation): boolean => {
	if (
		(observation.kind === "execution_error" ||
			observation.kind === "broker_rejection") &&
		(observation.action === "exit" || observation.action === "reduce")
	) {
		return true;
	}

	if (observation.kind === "final_outcome") {
		return true;
	}

	if (observation.kind === "execution" && observation.side === "sell") {
		return true;
	}

	if (
		observation.kind === "lifecycle_transition" &&
		typeof observation.status === "string"
	) {
		return auditExitLifecycle.has(observation.status);
	}

	return false;
};

/*
isAuditObservation keeps dashboard audit rows limited to entry and exit events so
the right rail can explain opens and closes without broker noise.
*/
export const isAuditObservation = (observation: TradeObservation): boolean =>
	isEntryObservation(observation) || isExitObservation(observation);

const auditReason = (observation: TradeObservation): string => {
	if (
		(observation.kind === "execution_error" ||
			observation.kind === "broker_rejection") &&
		typeof observation.error === "string" &&
		observation.error.length > 0
	) {
		return observation.error;
	}

	if (typeof observation.status === "string" && observation.status.length > 0) {
		return observation.status;
	}

	if (isExitObservation(observation)) {
		return "exit";
	}

	if (isEntryObservation(observation)) {
		return "enter";
	}

	return observation.kind.replaceAll("_", " ");
};

/*
tradeObservationAuditRow formats one immutable thesis trade journal row for the
dashboard audit rail using the same broker facts published by the backend.
*/
export const tradeObservationAuditRow = (
	observation: TradeObservation,
): AuditRow => {
	const timestamp =
		observation.at.length >= 19 ? observation.at.slice(11, 19) : observation.at;
	const identifier =
		observation.executionId ??
		observation.orderId ??
		String(observation.decision);
	const trade =
		observation.quantity && observation.price
			? `${observation.quantity} @ ${fixed(Number(observation.price))}`
			: "";
	const meta = [
		observation.kind,
		observation.action,
		observation.side,
		observation.symbol,
		trade,
		observation.error,
	].filter((value) => typeof value === "string" && value.length > 0);

	return {
		reason: auditReason(observation),
		reference: [`#${identifier}`, timestamp].filter(Boolean).join(" · "),
		meta: meta.join(" · "),
	};
};

/*
auditObservations retains only entry and exit journal rows for the dashboard
audit trail, in publication order.
 */
export const auditObservations = (
	observations: TradeObservation[],
): TradeObservation[] => observations.filter(isAuditObservation);

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

const AuditRows = ({
	observations,
	observed,
}: {
	observations: TradeObservation[];
	observed: boolean;
}) => (
	<div className="min-h-0 flex-1 overflow-auto py-0.5">
		{observations.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				{observed
					? "no trade activity yet"
					: "waiting for trade journal frames"}
			</div>
		) : null}
		{observations.map((observation) => {
			const row = tradeObservationAuditRow(observation);

			return (
				<div
					key={tradeObservationKey(observation)}
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
			<span ref={allowRef} /> active · <span ref={denyRef} /> passive
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
			const net = positionsStore.state.positions
				.filter((position) => symbols.includes(position.symbol))
				.reduce((sum, position) => sum + position.pnl, 0);

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
		(state) =>
			state.positions
				.filter(
					(position) =>
						position.qty > 0 &&
						Array.isArray(position.executions) &&
						position.executions.length > 0,
				)
				.map((position) => position.symbol),
		{ compare: sameSymbols },
	);
	const observed = useSelector(positionsStore, (state) => state.observed);
	const quoteSymbol = useSelector(
		positionsStore,
		(state) =>
			state.positions.find(
				(position) =>
					position.qty > 0 &&
					Array.isArray(position.executions) &&
					position.executions.length > 0,
			)?.symbol,
	);
	const quote = quoteSymbol?.split("/")[1] ?? "USD";
	const auditRows = useSelector(tradeJournalStore, (state) =>
		auditObservations(tradeJournalValues(state.journal)),
	);
	const journalObserved = useSelector(
		tradeJournalStore,
		(state) => state.observed,
	);

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
				<AuditRows
					observations={[...auditRows].reverse()}
					observed={journalObserved}
				/>
			</div>
		</div>
	);
};
