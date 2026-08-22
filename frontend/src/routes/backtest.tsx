import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useState } from "react";

import {
	appStore,
	type HindsightReport,
	type HindsightSignal,
	type HindsightSymbol,
} from "#/collections/app";
import { publishBacktestCommand } from "#/providers/websocket";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Stat } from "#/components/ui/stat";

const formatPct = (value: number): string => `${(value * 100).toFixed(2)}%`;

const formatClock = (iso: string | null): string => {
	if (iso === null) {
		return "--";
	}

	const parsed = new Date(iso);

	if (Number.isNaN(parsed.getTime())) {
		return "--";
	}

	return parsed.toLocaleTimeString([], { hour12: false });
};

/*
AlternativeBars renders the live measurement scores the logic was reading when
it declined a move, from the hottest to the coldest. Bars are relative to the
largest magnitude in the set so one glance shows which signal dominated —
exactly the "what prevented this move" answer the hindsight panel exists for.
*/
const AlternativeBars = ({
	alternatives,
}: {
	alternatives: Record<string, number>;
}) => {
	const entries = Object.entries(alternatives);
	const ceiling = Math.max(
		1e-9,
		...entries.map(([, score]) => Math.abs(score)),
	);

	return (
		<ul className="flex flex-col gap-0.5 pt-1">
			{entries.map(([source, score]) => {
				const clamp = Math.min(1, Math.abs(score) / ceiling);

				return (
					<li
						key={source}
						className="flex items-center gap-2 font-mono text-[10px]"
					>
						<span className="w-48 truncate text-(--f4)">{source}</span>
						<span
							className="h-1 shrink-0 rounded-full bg-(--acc) opacity-40"
							style={{ width: `${Math.max(3, clamp * 80)}px` }}
						/>
						<span className="tabular-nums text-(--f3)">{score.toFixed(4)}</span>
					</li>
				);
			})}
		</ul>
	);
};

/*
ThesisRow renders one labeled thesis measure compactly, or nothing when the
value is absent from the stored decision.
*/
const ThesisRow = ({ label, value }: { label: string; value?: number }) =>
	value === undefined ? null : (
		<div className="flex items-center gap-2 font-mono text-[10px]">
			<span className="w-40 text-(--f4)">{label}</span>
			<span className="tabular-nums text-(--f3)">{value.toFixed(4)}</span>
		</div>
	);

/*
JournalRows replays the audit decisions recorded between a missed hold's entry
and exit, so the why of staying flat is visible across the whole window rather
than only at the instant before entry.
*/
const JournalRows = ({ journal }: { journal?: HindsightSignal[] }) =>
	journal === undefined || journal.length === 0 ? null : (
		<div className="flex flex-col gap-1 border-t border-(--line) pt-1.5">
			<span className="font-mono text-[9px] uppercase tracking-widest text-(--f4)">
				audit between entry and exit
			</span>
			{journal.map((decision, index) => (
				<div
					key={`${decision.at}-${index}`}
					className="flex items-center gap-2 rounded-[3px] bg-(--sunken) px-2 py-1 font-mono text-[10px]"
				>
					<span className="tabular-nums text-(--f4)">
						{formatClock(decision.at)}
					</span>
					<span
						className={
							decision.action === "enter"
								? "text-(--up)"
								: decision.action === "exit" || decision.action === "reduce"
									? "text-(--warn)"
									: "text-(--f2)"
						}
					>
						{decision.action}
					</span>
					<span className="tabular-nums text-(--f3)">
						graph {decision.graphScore.toFixed(4)}
					</span>
					<span className="tabular-nums text-(--f3)">
						thesis {decision.thesisScore.toFixed(4)}
					</span>
					{decision.reason !== undefined && decision.reason !== "" ? (
						<span className="truncate text-(--f4)">{decision.reason}</span>
					) : null}
				</div>
			))}
		</div>
	);

/*
SymbolCard shows one symbol's hindsight: how much the tape offered, how much
the system collected, and each missed leg with the signal that declined it.
*/
const SymbolCard = ({ symbol }: { symbol: HindsightSymbol }) => {
	const [open, setOpen] = useState(false);

	return (
		<li className="border-b border-(--line) last:border-b-0">
			<Button
				variant="bare"
				className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-(--raised)"
				onClick={() => setOpen((previous) => !previous)}
			>
				<span className="w-32 truncate font-mono text-[11px] font-semibold text-(--f1)">
					{symbol.symbol}
				</span>
				<span className="w-20 font-mono text-[10px] tabular-nums text-(--f3)">
					{formatPct(symbol.upboundPct)}
				</span>
				<span className="w-28 font-mono text-[10px] tabular-nums text-(--down)">
					↓ missed {formatPct(symbol.missedPct)}
				</span>
				<span className="font-mono text-[10px] text-(--f4)">
					{symbol.missedLegs}/{symbol.legs} legs
				</span>
				<span className="ml-auto font-mono text-[9px] text-(--f4)">
					{open ? "▲" : "▼"}
				</span>
			</Button>

			{open ? (
				<div className="flex flex-col gap-2 border-t border-(--line) bg-(--sunken) px-4 py-3">
					{symbol.opportunities.length === 0 ? (
						<p className="font-mono text-[10px] text-(--f4)">
							No missed legs with actionable signal context.
						</p>
					) : null}
					{symbol.opportunities.slice(0, 6).map((opportunity) => (
						<div
							key={`${opportunity.leg.buyAt}-${opportunity.leg.sellAt}-${opportunity.leg.buyPrice}`}
							className="flex flex-col gap-1 rounded-[3px] border border-(--line) bg-(--surface) p-2.5"
						>
							<Flex.Row align="center" gap={2} className="font-mono text-[10px]">
								<span className="text-(--up)">↑</span>
								<span className="tabular-nums text-(--f2)">
									{opportunity.leg.buyPrice} → {opportunity.leg.sellPrice}
								</span>
								<span className="tabular-nums text-(--warn)">
									{formatPct(opportunity.leg.profitPct)} missed
								</span>
								<span className="ml-auto text-(--f4)">
									{formatClock(opportunity.leg.buyAt)} →{" "}
									{formatClock(opportunity.leg.sellAt)}
								</span>
							</Flex.Row>
							{opportunity.signal.opportunity ? (
								<span className="font-mono text-[10px] text-(--warn)">
									flagged {opportunity.signal.opportunityType} — decided nothing
								</span>
							) : null}
							{opportunity.why !== undefined && opportunity.why !== "" ? (
								<p className="rounded-[3px] border border-(--warn) bg-(--sunken) px-2 py-1 font-mono text-[10px] text-(--warn)">
									{opportunity.why}
								</p>
							) : null}
							<div className="flex flex-col gap-0.5 border-t border-(--line) pt-1.5">
								<ThesisRow
									label="thesis confidence"
									value={opportunity.signal.thesisConfidence}
								/>
								<ThesisRow
									label="thesis support"
									value={opportunity.signal.thesisSupport}
								/>
								<ThesisRow
									label="thesis contradiction"
									value={opportunity.signal.thesisContradiction}
								/>
								<ThesisRow
									label="thesis conditions"
									value={opportunity.signal.thesisConditions}
								/>
								<ThesisRow
									label="direction"
									value={opportunity.signal.direction}
								/>
								<ThesisRow
									label="admission threshold"
									value={opportunity.signal.admissionThreshold}
								/>
							</div>
							{opportunity.signal.reason !== undefined &&
							opportunity.signal.reason !== "" ? (
								<p className="font-mono text-[10px] text-(--f4)">
									{opportunity.signal.reason}
								</p>
							) : null}
							<JournalRows journal={opportunity.journal} />
							{opportunity.signal.alternatives !== null &&
							Object.keys(opportunity.signal.alternatives).length > 0 ? (
								<AlternativeBars
									alternatives={opportunity.signal.alternatives}
								/>
							) : null}
						</div>
					))}
				</div>
			) : null}
		</li>
	);
};

/*
HindsightPanel fills the right pane. The four summary stats sit in a border-
separated header strip so they are always visible while the symbol list scrolls.
*/
const HindsightPanel = ({ report }: { report: HindsightReport }) => (
	<Flex.Column className="min-h-0 flex-1 overflow-hidden">
		{/* Summary bar — always visible */}
		<div className="grid shrink-0 grid-cols-4 divide-x divide-(--line) border-b border-(--line)">
			<Stat
				layout="tile"
				label="perfect-execution ceiling"
				value={report.upboundPct.toFixed(4)}
				className="px-4 py-3"
			/>
			<Stat
				layout="tile"
				label="missed value"
				value={report.missedPct.toFixed(4)}
				variant="error"
				className="px-4 py-3"
			/>
			<Stat
				layout="tile"
				label="missed / total legs"
				value={`${report.missedLegs} / ${report.totalLegs}`}
				className="px-4 py-3"
			/>
			<Stat
				layout="tile"
				label="symbols on tape"
				value={report.symbols.length.toString()}
				className="px-4 py-3"
			/>
		</div>

		{/* Scrollable symbol list */}
		<ul className="min-h-0 flex-1 overflow-auto">
			{report.symbols
				.filter((symbol) => symbol.missedLegs > 0)
				.map((symbol) => (
					<SymbolCard key={symbol.symbol} symbol={symbol} />
				))}
		</ul>
	</Flex.Column>
);

/*
HindsightEmpty is the right-pane placeholder shown before a capture is loaded.
*/
const HindsightEmpty = ({ captureId }: { captureId: number | null }) => (
	<Flex.Column
		align="center"
		justify="center"
		gap={3}
		className="flex-1 p-8 text-center"
	>
		<span className="font-mono text-[10px] uppercase tracking-widest text-(--f4)">
			Hindsight
		</span>
		<p className="max-w-xs font-mono text-[11px] leading-relaxed text-(--f3)">
			{captureId === null || captureId === 0
				? "Select a capture from the left to analyze its tape — the panel will show the absolute maximum the tape contained and which signals kept the system out of each move."
				: "Capture loaded. Click Re-analyze to run hindsight."}
		</p>
	</Flex.Column>
);

const BacktestRoute = () => {
	const backtest = useSelector(appStore, (state) => state.backtest);
	const hindsight = backtest.hindsight;

	return (
		<div className="flex h-full min-w-275 overflow-hidden bg-(--bg)">
			{/* Left sidebar — capture list */}
			<Section
				fit="pane"
				surface="surface"
				className="w-72 shrink-0 border-r border-(--line)"
			>
				<Section.Header title="Captures" size="lg" rule sticky />
				<Section.Body>
					{backtest.captures.length === 0 ? (
						<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
							No captures yet — every live run records itself automatically.
						</p>
					) : null}
					<ul className="flex flex-col divide-y divide-(--line)">
						{backtest.captures.map((capture) => {
							const active = backtest.captureId === capture.id;

							return (
								<li key={capture.id}>
									<Button
										variant="bare"
										disabled={backtest.rebooting}
										className={`flex w-full flex-col items-start gap-1 px-3 py-2.5 text-left hover:bg-(--raised) ${active ? "bg-(--raised)" : ""}`}
										onClick={() => {
											publishBacktestCommand(
												"select",
												undefined,
												capture.id,
											);
										}}
									>
										<Flex.Row
											align="center"
											justify="between"
											className="w-full"
										>
											<span className="font-mono text-[11px] font-semibold tabular-nums text-(--f1)">
												#{capture.id}
											</span>
											{active ? (
												<span className="rounded-[3px] border border-(--acc) px-1.5 py-px font-mono text-[8px] uppercase tracking-widest text-(--acc)">
													{backtest.rebooting
														? "loading…"
														: backtest.playing
															? "playing"
															: "loaded"}
												</span>
											) : null}
										</Flex.Row>
										<span className="font-mono text-[10px] text-(--f4)">
											{new Date(capture.startedAt).toLocaleString()}
										</span>
										<span className="font-mono text-[10px] text-(--f4)">
											{capture.frames.toLocaleString()} frames
										</span>
									</Button>
								</li>
							);
						})}
					</ul>
				</Section.Body>
				<div className="shrink-0 border-t border-(--line) px-3 py-2">
					<p className="font-mono text-[10px] text-(--f4)">
						{backtest.captureId === null || backtest.captureId === 0
							? "Select a capture to load it, then press Play in the top bar."
							: backtest.rebooting
								? "Loading the session stream…"
								: backtest.playing
									? "Streaming capture frames from the store."
									: "Loaded. Press Play in the top bar to run the session."}
					</p>
				</div>
			</Section>

			{/* Right pane — hindsight */}
			<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-hidden">
				{/* Pane header */}
				<Flex.Row
					align="center"
					justify="between"
					className="h-11.5 shrink-0 border-b border-(--line) bg-(--surface) px-4"
				>
					<Flex.Row align="center" gap={3}>
						<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							Hindsight
						</span>
						<span className="font-mono text-[12px] font-semibold text-(--f1)">
							What the tape offered vs. what we took
						</span>
					</Flex.Row>
					{backtest.captureId !== null ? (
						<Button
							variant="outline"
							size="xs"
							onClick={() => {
								publishBacktestCommand(
									"hindsight",
									undefined,
									backtest.captureId ?? undefined,
								);
							}}
						>
							Re-analyze
						</Button>
					) : null}
				</Flex.Row>

				{/* Pane body */}
				{hindsight === null || hindsight.status === "analyzing" ? (
					<Flex.Column className="flex-1">
						<HindsightEmpty captureId={backtest.captureId} />
						{hindsight?.status === "analyzing" ? (
							<p className="shrink-0 border-t border-(--line) px-4 py-2.5 font-mono text-[10px] text-(--acc)">
								Analyzing capture #{hindsight.captureId}…
							</p>
						) : null}
					</Flex.Column>
				) : null}

				{hindsight !== null && hindsight.status === "error" ? (
					<Flex.Column className="flex-1 p-4">
						<p className="font-mono text-[10px] text-(--down)">
							Hindsight analysis failed for capture #{hindsight.captureId}.
						</p>
					</Flex.Column>
				) : null}

				{hindsight !== null && hindsight.status === "ready" ? (
					<HindsightPanel report={hindsight} />
				) : null}
			</Flex.Column>
		</div>
	);
};

export const Route = createFileRoute("/backtest")({
	component: BacktestRoute,
});
