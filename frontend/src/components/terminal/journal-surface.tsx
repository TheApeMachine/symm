import { useSelector } from "@tanstack/react-store";
import { useEffect } from "react";
import { positionStore, tradeHistoryStore } from "#/collections/app";
import type { TradeRecord } from "#/collections/types";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { Decision } from "#/providers/telemetry/telemetry/decision";
import { EntryCost } from "#/providers/telemetry/telemetry/entry-cost";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";
import { cn } from "@/lib/utils";

/*
journalBaseUrl locates the hub's REST endpoints, mirroring the websocket
origin (env override with a localhost default).
*/
const journalBaseUrl = () => {
	if (import.meta.env.VITE_SYMM_WS_URL) {
		return import.meta.env.VITE_SYMM_WS_URL.replace(/^ws/, "http").replace(
			/\/ws$/,
			"",
		);
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
		: typeof value === "string" &&
				value !== "" &&
				Number.isFinite(Number(value))
			? Number(value).toFixed(digits)
			: String(value ?? "—");

const formatPct = (value: unknown, digits: number): string => {
	const formatted = formatNumber(value, digits);
	return formatted === "—" ? formatted : `${formatted}%`;
};

const numberOf = (value: unknown): number => {
	if (typeof value === "number") return value;
	if (
		typeof value === "string" &&
		value !== "" &&
		Number.isFinite(Number(value))
	) {
		return Number(value);
	}
	return 0;
};

/* pnlTone maps a signed P&L to the terminal's up/down tone name. */
const pnlTone = (value: number): "up" | "down" | "f3" =>
	value > 0 ? "up" : value < 0 ? "down" : "f3";

const positionHolder = new Position();
const holdingHolder = new Holding();
const decisionHolder = new Decision();
const entryCostHolder = new EntryCost();

type ActiveLotEntry = {
	symbol: string;
	status: string;
	pnl: string;
	pnlValue: number;
	mark: string;
	returnPct: string;
	entryPrice: string;
	entryAt: string;
	opportunityType: string;
	opportunityPhase: string;
	predictiveStatus: string;
	confidence: string;
};

type JournalTradeEntry = {
	id: string;
	symbol: string;
	status: string;
	pnl: string;
	pnlValue: number;
	returnPct: string;
	entryPrice: string;
	entryAt: string;
	exitPrice: string;
	entryFee: string;
	exitFee: string;
	exitAt: string;
	exitAtSort: number;
	// Analysis fields sourced from Decision, not previously surfaced.
	bestAsk: string;
	bestBid: string;
	spread: string;
	impact: string;
	breakEven: string;
	roundTripFees: string;
	// What the system actually decided on: the opportunity it was positioning
	// for, how far the gate sequence got, and what the causal search concluded.
	opportunityType: string;
	opportunityPhase: string;
	predictiveStatus: string;
	confidence: string;
	source: "live" | "history";
};

const timeOf = (nanos: bigint | number | undefined): string => {
	const value =
		typeof nanos === "bigint"
			? Number(nanos / 1000000n)
			: (nanos ?? 0) / 1_000_000;
	return value > 0 ? new Date(value).toLocaleString() : "—";
};

const sortOf = (nanos: bigint | number | undefined): number =>
	typeof nanos === "bigint"
		? Number(nanos / 1000000n)
		: (nanos ?? 0) / 1_000_000;

/*
fromRecord builds a JournalTradeEntry from a persisted TradeRecord (GET
/trades, JSON-shaped like wire.PositionT). This is the historical path — it
survives restarts and is not bounded by the live ring buffer's 50-frame span.
*/
const fromRecord = (record: TradeRecord): JournalTradeEntry | null => {
	const holding = record.holding;
	if (!holding?.symbol) return null;

	const decision = record.decision;
	const entryCost = decision?.entryCost;
	const exitAtSort = holding.exitAt ?? 0;

	return {
		id: `${holding.symbol}-${String(exitAtSort)}-history`,
		symbol: holding.symbol,
		status: holding.status ?? record.status ?? "—",
		pnl: `${formatNumber(holding.pnl, 4)} USD`,
		pnlValue: numberOf(holding.pnl),
		returnPct: formatPct(holding.returnPct, 2),
		entryPrice: formatNumber(holding.entryPrice, 6),
		entryAt: timeOf(holding.entryAt),
		exitPrice: formatNumber(holding.exitPrice, 6),
		entryFee: formatNumber(holding.entryFee, 4),
		exitFee: formatNumber(holding.exitFee, 4),
		exitAt: timeOf(holding.exitAt),
		exitAtSort: sortOf(exitAtSort),
		bestAsk: formatNumber(entryCost?.bestAsk, 6),
		bestBid: formatNumber(entryCost?.bestBid, 6),
		spread: formatNumber(entryCost?.spread, 6),
		impact: formatNumber(entryCost?.impact, 6),
		breakEven: formatNumber(entryCost?.breakEven, 6),
		roundTripFees: formatNumber(entryCost?.roundTripFees, 6),
		opportunityType: decision?.opportunityType || "—",
		opportunityPhase: decision?.opportunityPhase || "—",
		predictiveStatus: decision?.predictiveStatus || "—",
		confidence: formatPct((decision?.confidence ?? 0) * 100, 1),
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

		const latestFrame = state.findLast(() => true);
		if (latestFrame) {
			for (let rowIndex = 0; rowIndex < latestFrame.rowsLength(); rowIndex++) {
				const currentPosition = latestFrame.rows(rowIndex, positionHolder);
				if (!currentPosition) continue;

				const currentHolding = currentPosition.holding(holdingHolder);
				if (!currentHolding) continue;

				const currentSymbol = currentHolding.symbol() ?? "";
				if (!currentSymbol) continue;

				const positionStatus =
					currentHolding.status() ?? currentPosition.status() ?? "—";
				if (positionStatus === "closed" || currentHolding.exitPrice()) continue;

				const currentDecision = currentPosition.decision(decisionHolder);

				const entryNano = currentHolding.entryAt();
				const entryTimestamp = entryNano > 0n ? timeOf(entryNano) : "—";

				activeMap.set(currentSymbol, {
					symbol: currentSymbol,
					status: positionStatus,
					pnl: `${formatNumber(currentHolding.pnl(), 4)} USD`,
					pnlValue: numberOf(currentHolding.pnl()),
					mark: formatNumber(currentHolding.mark(), 6),
					returnPct: formatPct(currentHolding.returnPct(), 2),
					entryPrice: formatNumber(currentHolding.entryPrice(), 6),
					entryAt: entryTimestamp,
					opportunityType: currentDecision?.opportunityType() || "—",
					opportunityPhase: currentDecision?.opportunityPhase() || "—",
					predictiveStatus: currentDecision?.predictiveStatus() || "—",
					confidence: formatPct((currentDecision?.confidence() ?? 0) * 100, 1),
				});
			}
		}

		for (const frame of state.toArray()) {
			for (let rowIndex = 0; rowIndex < frame.rowsLength(); rowIndex++) {
				const currentPosition = frame.rows(rowIndex, positionHolder);
				if (!currentPosition) continue;

				const currentHolding = currentPosition.holding(holdingHolder);
				if (!currentHolding) continue;

				const currentSymbol = currentHolding.symbol() ?? "";
				if (!currentSymbol) continue;

				const positionStatus =
					currentHolding.status() ?? currentPosition.status() ?? "—";
				const currentDecision = currentPosition.decision(decisionHolder);
				const currentEntryCost = currentDecision?.entryCost(entryCostHolder);

				const entryNano = currentHolding.entryAt();
				const entryTimestamp = entryNano > 0n ? timeOf(entryNano) : "—";

				if (positionStatus === "closed" || currentHolding.exitPrice()) {
					const exitNano = currentHolding.exitAt();
					const tradeId = `${currentSymbol}-${String(exitNano)}`;
					closedMap.set(tradeId, {
						id: tradeId,
						symbol: currentSymbol,
						status: positionStatus,
						pnl: `${formatNumber(currentHolding.pnl(), 4)} USD`,
						pnlValue: numberOf(currentHolding.pnl()),
						returnPct: formatPct(currentHolding.returnPct(), 2),
						entryPrice: formatNumber(currentHolding.entryPrice(), 6),
						entryAt: entryTimestamp,
						exitPrice: formatNumber(currentHolding.exitPrice(), 6),
						entryFee: formatNumber(currentHolding.entryFee(), 4),
						exitFee: formatNumber(currentHolding.exitFee(), 4),
						exitAt: timeOf(exitNano),
						exitAtSort: sortOf(exitNano),
						bestAsk: formatNumber(currentEntryCost?.bestAsk(), 6),
						bestBid: formatNumber(currentEntryCost?.bestBid(), 6),
						spread: formatNumber(currentEntryCost?.spread(), 6),
						impact: formatNumber(currentEntryCost?.impact(), 6),
						breakEven: formatNumber(currentEntryCost?.breakEven(), 6),
						roundTripFees: formatNumber(currentEntryCost?.roundTripFees(), 6),
						opportunityType: currentDecision?.opportunityType() || "—",
						opportunityPhase: currentDecision?.opportunityPhase() || "—",
						predictiveStatus: currentDecision?.predictiveStatus() || "—",
						confidence: formatPct(
							(currentDecision?.confidence() ?? 0) * 100,
							1,
						),
						source: "live",
					});
				}
			}
		}

		// Merge in persisted history, deduped against live frames by symbol +
		// exit timestamp so a trade that's in both sources renders once — the
		// live copy wins since it carries the freshest decision snapshot.
		const liveExitKeys = new Set(
			[...closedMap.values()].map(
				(entry) => `${entry.symbol}-${entry.exitAtSort}`,
			),
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
			closedTrades: [...closedMap.values()].sort(
				(left, right) => right.exitAtSort - left.exitAtSort,
			),
		};
	});

	return (
		<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(280px,320px)_minmax(420px,1fr)]">
			<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
				<Section>
					<Section.Header
						title="Lifecycle rail"
						meta={`${activeLots.length} lots`}
					/>
					<div className="flex flex-col gap-2 p-2">
						{activeLots.length === 0 ? (
							<Panel
								variant="surface"
								size="bare"
								className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
							>
								no lots held
							</Panel>
						) : (
							activeLots.map((lot) => {
								const tone = pnlTone(lot.pnlValue);
								return (
									<Panel
										key={lot.symbol}
										variant="surface"
										size="bare"
										className={cn(
											"flex flex-col gap-1.5 border-l-2 px-2.5 py-2 font-mono text-[11px]",
											tone === "up" && "border-l-(--up)",
											tone === "down" && "border-l-(--down)",
											tone === "f3" && "border-l-(--line2)",
										)}
									>
										<Flex.Row className="items-center justify-between gap-2">
											<Flex.Column className="min-w-0 gap-0.5">
												<Typography.Span className="truncate font-semibold text-[12.5px] text-(--f1)">
													{lot.symbol}
												</Typography.Span>
												<Typography.Span className="text-[9.5px] text-(--f4)">
													entry {lot.entryPrice} · mark {lot.mark}
												</Typography.Span>
											</Flex.Column>
											<Flex.Column className="shrink-0 items-end gap-0.5">
												<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase tracking-wide text-(--f3)">
													{lot.status}
												</Typography.Span>
												<Typography.Mono size="s" tone={tone} weight="semibold">
													{lot.pnl} ({lot.returnPct})
												</Typography.Mono>
											</Flex.Column>
										</Flex.Row>
										<Flex.Row className="items-center justify-between gap-2 text-[9px] text-(--f4)">
											<Typography.Span>entered {lot.entryAt}</Typography.Span>
										</Flex.Row>
										<Flex.Row className="items-center justify-between gap-2 border-(--line) border-t pt-1 text-[9px] text-(--f4)">
											<Typography.Span className="truncate">
												{lot.opportunityType} · {lot.opportunityPhase} (
												{lot.confidence})
											</Typography.Span>
										</Flex.Row>
									</Panel>
								);
							})
						)}
					</div>
				</Section>
			</div>

			<div className="min-h-0 overflow-auto px-4 py-4.5">
				<Section.Header
					size="bare"
					rule={false}
					title="Journal"
					meta={`${closedTrades.length} entries`}
				/>
				<div className="mt-2 flex flex-col gap-2">
					{closedTrades.length === 0 ? (
						<Panel
							variant="surface"
							size="bare"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							nothing traded yet
						</Panel>
					) : (
						closedTrades.map((trade) => {
							const tone = pnlTone(trade.pnlValue);
							return (
								<Panel
									key={trade.id}
									variant="surface"
									size="bare"
									className={cn(
										"flex flex-col gap-2 border-l-2 p-3 font-mono text-[11px]",
										tone === "up" && "border-l-(--up)",
										tone === "down" && "border-l-(--down)",
										tone === "f3" && "border-l-(--line2)",
									)}
								>
									{/* Primary: what happened. */}
									<Flex.Row className="items-center justify-between gap-2">
										<Flex.Row className="items-center gap-2">
											<Typography.Span className="font-semibold text-[13px] text-(--f1)">
												{trade.symbol}
											</Typography.Span>
											{trade.source === "history" ? (
												<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase text-(--f4)">
													history
												</Typography.Span>
											) : null}
										</Flex.Row>
										<Flex.Row className="items-baseline gap-2">
											<Typography.Mono size="lg" tone={tone} weight="semibold">
												{trade.pnl}
											</Typography.Mono>
											<Typography.Mono size="s" tone={tone}>
												({trade.returnPct})
											</Typography.Mono>
										</Flex.Row>
									</Flex.Row>

									{/* Secondary: prices, timing, fees. */}
									<Flex.Row className="items-center justify-between text-[9.5px] text-(--f2)">
										<Typography.Span>
											{trade.entryPrice} → {trade.exitPrice}
										</Typography.Span>
										<Typography.Span className="text-(--f4)">
											entered {trade.entryAt} · exited {trade.exitAt}
										</Typography.Span>
									</Flex.Row>
									<Flex.Row className="items-center justify-between border-(--line) border-t pt-1.5 text-[9px] text-(--f4)">
										<Typography.Span>
											fees {trade.entryFee} → {trade.exitFee}
										</Typography.Span>
									</Flex.Row>

									{/* Diagnostics: why the model did this, demoted a step further. */}
									<div className="flex flex-col gap-1 rounded-xs bg-(--sunken) px-2 py-1.5 text-[9px] text-(--f4)">
										<Flex.Row className="items-center justify-between">
											<Typography.Span>
												opportunity{" "}
												<span className="text-(--f3)">
													{trade.opportunityType}
												</span>{" "}
												({trade.opportunityPhase}) · {trade.predictiveStatus}
											</Typography.Span>
											<Typography.Span>conf {trade.confidence}</Typography.Span>
										</Flex.Row>
										<Flex.Row className="items-center justify-between">
											<Typography.Span>
												spread {trade.spread} · impact {trade.impact}
											</Typography.Span>
											<Typography.Span>
												round-trip fees {trade.roundTripFees}
											</Typography.Span>
										</Flex.Row>
										<Flex.Row className="items-center justify-between">
											<Typography.Span>
												best {trade.bestBid} / {trade.bestAsk} · BE{" "}
												{trade.breakEven}
											</Typography.Span>
										</Flex.Row>
									</div>
								</Panel>
							);
						})
					)}
				</div>
			</div>
		</div>
	);
};
