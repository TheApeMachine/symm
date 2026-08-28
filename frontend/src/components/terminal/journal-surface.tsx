import { useSelector } from "@tanstack/react-store";
import { useEffect } from "react";
import type { TradeRecord } from "#/collections/app";
import { positionStore, tradeHistoryStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { Decision } from "#/providers/telemetry/telemetry/decision";
import { DecisionTrace } from "#/providers/telemetry/telemetry/decision-trace";
import { EntryCost } from "#/providers/telemetry/telemetry/entry-cost";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";
import { RiskPlan } from "#/providers/telemetry/telemetry/risk-plan";
import { Stoploss } from "#/providers/telemetry/telemetry/stoploss";

/*
journalBaseUrl locates the hub's REST endpoints, mirroring the websocket
origin (env override with a localhost default) — same derivation as
routes/backtest.tsx's backtestBaseUrl.
*/
const journalBaseUrl = () => {
	if (import.meta.env.VITE_SYMM_WS_URL) {
		return import.meta.env.VITE_SYMM_WS_URL.replace(/^ws/, "http").replace(/\/ws$/, "");
	}

	const protocol = window.location.protocol === "https:" ? "https:" : "http:";
	const host =
		!window.location.hostname || window.location.hostname === "localhost"
			? "127.0.0.1"
			: window.location.hostname;

	return `${protocol}//${host}:8765`;
};

const formatNumber = (value: unknown, digits: number): string =>
	typeof value === "number"
		? value.toFixed(digits)
		: typeof value === "string" && value !== "" && Number.isFinite(Number(value))
			? Number(value).toFixed(digits)
			: String(value ?? "—");

const formatPct = (value: unknown, digits: number): string => {
	const formatted = formatNumber(value, digits);
	return formatted === "—" ? formatted : `${formatted}%`;
};

const positionHolder = new Position();
const holdingHolder = new Holding();
const stoplossHolder = new Stoploss();
const decisionHolder = new Decision();
const entryCostHolder = new EntryCost();
const riskHolder = new RiskPlan();
const traceHolder = new DecisionTrace();

type ActiveLotEntry = {
	symbol: string;
	status: string;
	stopStatus: string;
	pnl: string;
	mark: string;
	returnPct: string;
	entryPrice: string;
	entryAt: string;
	floor: string;
	peak: string;
	locked: boolean;
	surgeArmed: boolean;
	thesisScore: string;
	thesisConfidence: string;
	riskDistance: string;
	trailDistance: string;
};

type JournalTradeEntry = {
	id: string;
	symbol: string;
	status: string;
	pnl: string;
	returnPct: string;
	entryPrice: string;
	entryAt: string;
	exitPrice: string;
	entryFee: string;
	exitFee: string;
	triggerReason: string;
	stopStatus: string;
	locked: boolean;
	surgeArmed: boolean;
	exitAt: string;
	exitAtSort: number;
	// Analysis fields sourced from Decision, not previously surfaced.
	thesisScore: string;
	thesisConfidence: string;
	causalIdentification: string;
	allocationHaircut: string;
	allocationHaircutReason: string;
	adverseSelection: string;
	expectedReturn: string;
	expectedFees: string;
	bestAsk: string;
	bestBid: string;
	spread: string;
	impact: string;
	breakEven: string;
	riskDistance: string;
	trailDistance: string;
	maxLoss: string;
	hypothesis: string;
	recommendedAction: string;
	graphSupports: string;
	graphContradicts: string;
	source: "live" | "history";
};

const timeOf = (nanos: bigint | number | undefined): string => {
	const value = typeof nanos === "bigint" ? Number(nanos / 1000000n) : (nanos ?? 0) / 1_000_000;
	return value > 0 ? new Date(value).toLocaleString() : "—";
};

const sortOf = (nanos: bigint | number | undefined): number =>
	typeof nanos === "bigint" ? Number(nanos / 1000000n) : (nanos ?? 0) / 1_000_000;

/*
fromRecord builds a JournalTradeEntry from a persisted TradeRecord (GET
/trades, JSON-shaped like wire.PositionT). This is the historical path — it
survives restarts and is not bounded by the live ring buffer's 50-frame span.
*/
const fromRecord = (record: TradeRecord): JournalTradeEntry | null => {
	const holding = record.holding;
	if (!holding?.symbol) return null;

	const stoploss = holding.stoploss;
	const decision = record.decision;
	const entryCost = decision?.entryCost;
	const risk = decision?.risk;
	const trace = decision?.trace;
	const exitAtSort = holding.exitAt ?? 0;

	return {
		id: `${holding.symbol}-${String(exitAtSort)}-history`,
		symbol: holding.symbol,
		status: holding.status ?? record.status ?? "—",
		pnl: `${formatNumber(holding.pnl, 4)} USD`,
		returnPct: formatPct(holding.returnPct, 2),
		entryPrice: formatNumber(holding.entryPrice, 6),
		entryAt: timeOf(holding.entryAt),
		exitPrice: formatNumber(holding.exitPrice, 6),
		entryFee: formatNumber(holding.entryFee, 4),
		exitFee: formatNumber(holding.exitFee, 4),
		triggerReason: stoploss?.triggerReason ?? "—",
		stopStatus: stoploss?.status ?? "—",
		locked: stoploss?.locked ?? false,
		surgeArmed: stoploss?.surgeArmed ?? false,
		exitAt: timeOf(holding.exitAt),
		exitAtSort: sortOf(exitAtSort),
		thesisScore: formatNumber(decision?.thesisScore, 3),
		thesisConfidence: formatPct((decision?.thesisConfidence ?? 0) * 100, 1),
		causalIdentification: decision?.causalIdentification || "—",
		allocationHaircut: formatPct((decision?.allocationHaircut ?? 0) * 100, 1),
		allocationHaircutReason: decision?.allocationHaircutReason || "—",
		adverseSelection: formatNumber(decision?.adverseSelection, 6),
		expectedReturn: formatNumber(decision?.expectedReturn, 6),
		expectedFees: formatNumber(decision?.expectedFees, 6),
		bestAsk: formatNumber(entryCost?.bestAsk, 6),
		bestBid: formatNumber(entryCost?.bestBid, 6),
		spread: formatNumber(entryCost?.spread, 6),
		impact: formatNumber(entryCost?.impact, 6),
		breakEven: formatNumber(entryCost?.breakEven, 6),
		riskDistance: formatNumber(risk?.riskDistance, 6),
		trailDistance: formatNumber(risk?.trailDistance, 6),
		maxLoss: formatNumber(risk?.maxLoss, 4),
		hypothesis: trace?.hypothesis || "—",
		recommendedAction: trace?.recommendedAction || "—",
		graphSupports: formatNumber(trace?.graphSupports, 0),
		graphContradicts: formatNumber(trace?.graphContradicts, 0),
		source: "history",
	};
};

/*
JournalSurface is the record of what the desk actually did. Closed trades are
merged from two sources: the live positionStore ring buffer (frames still
in the last-50-publishes window) and tradeHistoryStore (the full persisted
position_trades table, fetched once on mount from GET /trades) — so a trade
survives on screen long after its live frame has been evicted, and history
from prior sessions shows up immediately.
*/
export const JournalSurface = () => {
	useEffect(() => {
		let cancelled = false;

		const loadHistory = async () => {
			try {
				const response = await fetch(`${journalBaseUrl()}/trades?limit=200`);
				if (!response.ok) return;

				const trades = (await response.json()) as TradeRecord[] | null;
				if (!cancelled && trades) {
					tradeHistoryStore.setState(() => trades);
				}
			} catch {
				// The hub may still be booting; a later mount re-fetches.
			}
		};

		loadHistory();

		return () => {
			cancelled = true;
		};
	}, []);

	const history = useSelector(tradeHistoryStore, (state) => state);

	const { activeLots, closedTrades } = useSelector(positionStore, (state) => {
		const activeMap = new Map<string, ActiveLotEntry>();
		const closedMap = new Map<string, JournalTradeEntry>();

		for (const frame of state.toArray()) {
			for (let rowIndex = 0; rowIndex < frame.rowsLength(); rowIndex++) {
				const currentPosition = frame.rows(rowIndex, positionHolder);
				if (!currentPosition) continue;

				const currentHolding = currentPosition.holding(holdingHolder);
				if (!currentHolding) continue;

				const currentSymbol = currentHolding.symbol() ?? "";
				if (!currentSymbol) continue;

				const positionStatus = currentHolding.status() ?? currentPosition.status() ?? "—";
				const currentStoploss = currentHolding.stoploss(stoplossHolder);
				const currentDecision = currentPosition.decision(decisionHolder);
				const currentEntryCost = currentDecision?.entryCost(entryCostHolder);
				const currentRisk = currentDecision?.risk(riskHolder);
				const currentTrace = currentDecision?.trace(traceHolder);

				const entryNano = currentHolding.entryAt();
				const entryTimestamp = entryNano > 0n ? timeOf(entryNano) : "—";

				if (positionStatus === "closed" || currentHolding.exitPrice()) {
					activeMap.delete(currentSymbol);

					const exitNano = currentHolding.exitAt();
					const tradeId = `${currentSymbol}-${String(exitNano)}`;
					closedMap.set(tradeId, {
						id: tradeId,
						symbol: currentSymbol,
						status: positionStatus,
						pnl: `${formatNumber(currentHolding.pnl(), 4)} USD`,
						returnPct: formatPct(currentHolding.returnPct(), 2),
						entryPrice: formatNumber(currentHolding.entryPrice(), 6),
						entryAt: entryTimestamp,
						exitPrice: formatNumber(currentHolding.exitPrice(), 6),
						entryFee: formatNumber(currentHolding.entryFee(), 4),
						exitFee: formatNumber(currentHolding.exitFee(), 4),
						triggerReason: currentStoploss?.triggerReason() ?? "—",
						stopStatus: currentStoploss?.status() ?? "—",
						locked: currentStoploss?.locked() ?? false,
						surgeArmed: currentStoploss?.surgeArmed() ?? false,
						exitAt: timeOf(exitNano),
						exitAtSort: sortOf(exitNano),
						thesisScore: formatNumber(currentDecision?.thesisScore(), 3),
						thesisConfidence: formatPct((currentDecision?.thesisConfidence() ?? 0) * 100, 1),
						causalIdentification: currentDecision?.causalIdentification() || "—",
						allocationHaircut: formatPct((currentDecision?.allocationHaircut() ?? 0) * 100, 1),
						allocationHaircutReason: currentDecision?.allocationHaircutReason() || "—",
						adverseSelection: formatNumber(currentDecision?.adverseSelection(), 6),
						expectedReturn: formatNumber(currentDecision?.expectedReturn(), 6),
						expectedFees: formatNumber(currentDecision?.expectedFees(), 6),
						bestAsk: formatNumber(currentEntryCost?.bestAsk(), 6),
						bestBid: formatNumber(currentEntryCost?.bestBid(), 6),
						spread: formatNumber(currentEntryCost?.spread(), 6),
						impact: formatNumber(currentEntryCost?.impact(), 6),
						breakEven: formatNumber(currentEntryCost?.breakEven(), 6),
						riskDistance: formatNumber(currentRisk?.riskDistance(), 6),
						trailDistance: formatNumber(currentRisk?.trailDistance(), 6),
						maxLoss: formatNumber(currentRisk?.maxLoss(), 4),
						hypothesis: currentTrace?.hypothesis() || "—",
						recommendedAction: currentTrace?.recommendedAction() || "—",
						graphSupports: formatNumber(currentTrace?.graphSupports(), 0),
						graphContradicts: formatNumber(currentTrace?.graphContradicts(), 0),
						source: "live",
					});
					continue;
				}

				activeMap.set(currentSymbol, {
					symbol: currentSymbol,
					status: positionStatus,
					stopStatus: currentStoploss?.status() ?? "—",
					pnl: `${formatNumber(currentHolding.pnl(), 4)} USD`,
					mark: formatNumber(currentHolding.mark(), 6),
					returnPct: formatPct(currentHolding.returnPct(), 2),
					entryPrice: formatNumber(currentHolding.entryPrice(), 6),
					entryAt: entryTimestamp,
					floor: formatNumber(currentStoploss?.floor(), 6),
					peak: formatNumber(currentStoploss?.peak(), 6),
					locked: currentStoploss?.locked() ?? false,
					surgeArmed: currentStoploss?.surgeArmed() ?? false,
					thesisScore: formatNumber(currentDecision?.thesisScore(), 3),
					thesisConfidence: formatPct((currentDecision?.thesisConfidence() ?? 0) * 100, 1),
					riskDistance: formatNumber(currentRisk?.riskDistance(), 6),
					trailDistance: formatNumber(currentRisk?.trailDistance(), 6),
				});
			}
		}

		// Merge in persisted history, deduped against live frames by symbol +
		// exit timestamp so a trade that's in both sources renders once — the
		// live copy wins since it carries the freshest decision snapshot.
		const liveExitKeys = new Set(
			[...closedMap.values()].map((entry) => `${entry.symbol}-${entry.exitAtSort}`),
		);

		for (const record of history) {
			const entry = fromRecord(record);
			if (!entry) continue;

			if (!liveExitKeys.has(`${entry.symbol}-${entry.exitAtSort}`)) {
				closedMap.set(entry.id, entry);
			}
		}

		return {
			activeLots: [...activeMap.values()].sort((leftLot, rightLot) =>
				leftLot.symbol.localeCompare(rightLot.symbol),
			),
			closedTrades: [...closedMap.values()].sort((left, right) => right.exitAtSort - left.exitAtSort),
		};
	});

	return (
		<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(280px,320px)_minmax(420px,1fr)]">
			<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
				<Section>
					<Section.Header title="Lifecycle rail" meta={`${activeLots.length} lots`} />
					<div className="flex flex-col gap-2 p-2">
						{activeLots.length === 0 ? (
							<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
								no lots held
							</Panel>
						) : (
							activeLots.map((lot) => (
								<Panel key={lot.symbol} variant="surface" size="bare" className="flex flex-col gap-1 px-2.5 py-2 font-mono text-[11px]">
									<Flex.Row className="items-center justify-between gap-2">
										<Flex.Column className="min-w-0 gap-0.5">
											<Typography.Span className="truncate font-semibold text-(--f1)">
												{lot.symbol}
											</Typography.Span>
											<Typography.Span className="text-[9.5px] text-(--f4)">
												entry {lot.entryPrice} · mark {lot.mark}
											</Typography.Span>
										</Flex.Column>
										<Flex.Column className="shrink-0 items-end gap-0.5">
											<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase tracking-wide">
												{lot.status}
											</Typography.Span>
											<Typography.Span className="text-[9.5px] text-(--pnl)">
												{lot.pnl} ({lot.returnPct})
											</Typography.Span>
										</Flex.Column>
									</Flex.Row>
									<Flex.Row className="items-center justify-between gap-2 border-(--line) border-t pt-1 text-[9px] text-(--f4)">
										<Typography.Span
											className={lot.stopStatus === "error" ? "font-semibold uppercase text-(--down)" : "uppercase"}
										>
											{lot.stopStatus === "error" ? "⚠ stop error" : `stop ${lot.stopStatus}`}
										</Typography.Span>
										<Typography.Span>
											floor {lot.floor} · peak {lot.peak}
										</Typography.Span>
									</Flex.Row>
									<Flex.Row className="items-center justify-between gap-2 text-[9px] text-(--f4)">
										<Typography.Span>entered {lot.entryAt}</Typography.Span>
										<Typography.Span>
											{lot.locked ? "locked" : "unlocked"}
											{lot.surgeArmed ? " · surge" : ""}
										</Typography.Span>
									</Flex.Row>
									<Flex.Row className="items-center justify-between gap-2 border-(--line) border-t pt-1 text-[9px] text-(--f4)">
										<Typography.Span>
											thesis {lot.thesisScore} ({lot.thesisConfidence})
										</Typography.Span>
										<Typography.Span>
											risk {lot.riskDistance} · trail {lot.trailDistance}
										</Typography.Span>
									</Flex.Row>
								</Panel>
							))
						)}
					</div>
				</Section>
			</div>

			<div className="min-h-0 overflow-auto px-4 py-4.5">
				<Section.Header size="bare" rule={false} title="Journal" meta={`${closedTrades.length} entries`} />
				<div className="mt-2 flex flex-col gap-2">
					{closedTrades.length === 0 ? (
						<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
							nothing traded yet
						</Panel>
					) : (
						closedTrades.map((trade) => (
							<Panel key={trade.id} variant="surface" size="bare" className="flex flex-col gap-1.5 p-3 font-mono text-[11px]">
								<Flex.Row className="items-center justify-between gap-2">
									<Flex.Row className="items-center gap-2">
										<Typography.Span className="font-semibold text-[12px] text-(--f1)">
											{trade.symbol}
										</Typography.Span>
										<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8.5px] uppercase text-(--f3)">
											{trade.triggerReason}
										</Typography.Span>
										{trade.source === "history" ? (
											<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase text-(--f4)">
												history
											</Typography.Span>
										) : null}
									</Flex.Row>
									<Flex.Row className="items-center gap-2">
										<Typography.Span className="font-semibold text-(--pnl)">
											{trade.pnl}
										</Typography.Span>
										<Typography.Span className="text-[9.5px] text-(--pnl)">
											({trade.returnPct})
										</Typography.Span>
									</Flex.Row>
								</Flex.Row>
								<Flex.Row className="items-center justify-between text-[9.5px] text-(--f4)">
									<Typography.Span>
										{trade.entryPrice} → {trade.exitPrice}
									</Typography.Span>
									<Typography.Span>
										entered {trade.entryAt} · exited {trade.exitAt}
									</Typography.Span>
								</Flex.Row>
								<Flex.Row className="items-center justify-between border-(--line) border-t pt-1 text-[9px] text-(--f4)">
									<Typography.Span>
										fees {trade.entryFee} → {trade.exitFee}
									</Typography.Span>
									<Typography.Span>
										<Typography.Span
											className={trade.stopStatus === "error" ? "font-semibold uppercase text-(--down)" : "uppercase"}
										>
											{trade.stopStatus === "error" ? "⚠ stop error" : trade.stopStatus}
										</Typography.Span>
										{trade.locked ? " · locked" : ""}
										{trade.surgeArmed ? " · surge" : ""}
									</Typography.Span>
								</Flex.Row>
								<Flex.Row className="items-center justify-between border-(--line) border-t pt-1 text-[9px] text-(--f4)">
									<Typography.Span>
										thesis {trade.thesisScore} ({trade.thesisConfidence}) · causal {trade.causalIdentification}
									</Typography.Span>
									<Typography.Span>
										haircut {trade.allocationHaircut}
										{trade.allocationHaircutReason !== "—" ? ` (${trade.allocationHaircutReason})` : ""}
									</Typography.Span>
								</Flex.Row>
								<Flex.Row className="items-center justify-between text-[9px] text-(--f4)">
									<Typography.Span>
										spread {trade.spread} · impact {trade.impact} · adverse {trade.adverseSelection}
									</Typography.Span>
									<Typography.Span>
										expected {trade.expectedReturn} ({trade.expectedFees} fees)
									</Typography.Span>
								</Flex.Row>
								<Flex.Row className="items-center justify-between text-[9px] text-(--f4)">
									<Typography.Span>
										risk {trade.riskDistance} · trail {trade.trailDistance} · max loss {trade.maxLoss}
									</Typography.Span>
									<Typography.Span>
										best {trade.bestBid} / {trade.bestAsk} · BE {trade.breakEven}
									</Typography.Span>
								</Flex.Row>
								{trade.hypothesis !== "—" || trade.recommendedAction !== "—" ? (
									<Flex.Row className="items-center justify-between text-[9px] text-(--f4)">
										<Typography.Span className="truncate">hypothesis: {trade.hypothesis}</Typography.Span>
										<Typography.Span>
											mcts: {trade.recommendedAction} (+{trade.graphSupports}/-{trade.graphContradicts})
										</Typography.Span>
									</Flex.Row>
								) : null}
							</Panel>
						))
					)}
				</div>
			</div>
		</div>
	);
};
