import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useState } from "react";

import {
	appStore,
	type HindsightReport,
	type HindsightSymbol,
} from "#/collections/app";
import { publishBacktestCommand } from "#/providers/websocket";
import { Section } from "@/components/ui/section";

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
		<ul className="flex flex-col gap-0.5">
			{entries.map(([source, score]) => {
				const clamp = Math.min(1, Math.abs(score) / ceiling);

				return (
					<li
						key={source}
						className="flex items-center gap-2 text-[11px]"
					>
						<span className="w-56 truncate text-muted-foreground">
							{source}
						</span>
						<span
							className="h-1.5 rounded-full bg-current opacity-60"
							style={{ width: `${Math.max(3, clamp * 100)}%` }}
						/>
						<span className="tabular-nums">{score.toFixed(4)}</span>
					</li>
				);
			})}
		</ul>
	);
};

/*
SymbolCard shows one symbol's hindsight: how much the tape offered, how much
the system collected, and each missed leg with the signal that declined it.
*/
const SymbolCard = ({ symbol }: { symbol: HindsightSymbol }) => {
	const [open, setOpen] = useState(false);

	return (
		<li className="rounded border">
			<button
				type="button"
				className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-white/5"
				onClick={() => setOpen((previous) => !previous)}
			>
				<span className="w-28 truncate font-medium">{symbol.symbol}</span>
				<span className="w-20 tabular-nums text-muted-foreground">
					{formatPct(symbol.upboundPct)}
				</span>
				<span className="w-24 tabular-nums text-rose-400">
					missed {formatPct(symbol.missedPct)}
				</span>
				<span className="text-muted-foreground">
					{symbol.missedLegs}/{symbol.legs} legs
				</span>
			</button>
			{open ? (
				<div className="flex flex-col gap-1 border-t px-3 py-2">
					{symbol.opportunities.length === 0 ? (
						<p className="text-xs text-muted-foreground">
							No missed legs with actionable signal context.
						</p>
					) : null}
					{symbol.opportunities.slice(0, 6).map((opportunity) => (
						<div
							key={`${opportunity.leg.buyAt}-${opportunity.leg.sellAt}-${opportunity.leg.buyPrice}`}
							className="flex flex-col gap-1 py-1"
						>
							<div className="flex items-center gap-2 text-xs">
								<span className="text-emerald-400">↑</span>
								<span className="tabular-nums">
									{opportunity.leg.buyPrice} →{" "}
									{opportunity.leg.sellPrice}
								</span>
								<span className="tabular-nums text-amber-400">
									{formatPct(opportunity.leg.profitPct)} missed
								</span>
								<span className="ml-auto text-muted-foreground">
									{formatClock(opportunity.leg.buyAt)} →{" "}
									{formatClock(opportunity.leg.sellAt)}
								</span>
							</div>
							{opportunity.signal.opportunity ? (
								<span className="text-[11px] text-amber-300">
									flagged {opportunity.signal.opportunityType} —
									decided nothing
								</span>
							) : null}
							{opportunity.signal.alternatives !== null &&
							Object.keys(opportunity.signal.alternatives).length >
								0 ? (
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
HindsightPanel shows the perfect-execution analysis for the loaded capture.
It reports the tape's theoretical maximum against what the system collected,
then each symbol's missed opportunity with the signal that declined it.
*/
const HindsightPanel = ({ report }: { report: HindsightReport }) => (
	<div className="flex flex-col gap-3">
		<div className="grid grid-cols-4 gap-2 text-center">
			<div className="rounded border px-2 py-1.5">
				<div className="text-sm font-semibold tabular-nums">
					{report.upboundPct.toFixed(4)}
				</div>
				<div className="text-[11px] text-muted-foreground">
					perfect-execution ceiling
				</div>
			</div>
			<div className="rounded border px-2 py-1.5">
				<div className="text-sm font-semibold tabular-nums text-rose-400">
					{report.missedPct.toFixed(4)}
				</div>
				<div className="text-[11px] text-muted-foreground">
					missed value
				</div>
			</div>
			<div className="rounded border px-2 py-1.5">
				<div className="text-sm font-semibold tabular-nums">
					{report.missedLegs}/{report.totalLegs}
				</div>
				<div className="text-[11px] text-muted-foreground">
					missed / total legs
				</div>
			</div>
			<div className="rounded border px-2 py-1.5">
				<div className="text-sm font-semibold tabular-nums">
					{report.symbols.length}
				</div>
				<div className="text-[11px] text-muted-foreground">
					symbols on tape
				</div>
			</div>
		</div>
		<ul className="flex flex-col gap-1">
			{report.symbols
				.filter((symbol) => symbol.missedLegs > 0)
				.map((symbol) => (
					<SymbolCard key={symbol.symbol} symbol={symbol} />
				))}
		</ul>
	</div>
);

const BacktestRoute = () => {
	const backtest = useSelector(appStore, (state) => state.backtest);
	const hindsight = backtest.hindsight;

	return (
		<Section className="p-4">
			<h2 className="text-base font-semibold">Captured sessions</h2>
			{backtest.captures.length === 0 ? (
				<p className="text-sm text-muted-foreground">
					No captures yet — every live run records itself into the
					capture store automatically.
				</p>
			) : null}
			<ul className="mt-2 flex flex-col gap-1">
				{backtest.captures.map((capture) => (
					<li key={capture.id}>
						<button
							type="button"
							className="w-full rounded border px-3 py-2 text-left text-sm hover:bg-white/10"
							disabled={backtest.rebooting}
							onClick={() => {
								publishBacktestCommand(
									"select",
									undefined,
									capture.id,
								);
							}}
						>
							<span className="tabular-nums">#{capture.id}</span>{" "}
							{new Date(capture.startedAt).toLocaleString()} ·{" "}
							{capture.frames.toLocaleString()} frames
							{backtest.captureId === capture.id ? " · loaded" : ""}
						</button>
					</li>
				))}
			</ul>
			<p className="mt-3 text-sm text-muted-foreground">
				Playback controls live in the top bar and work from every
				surface.
			</p>

			<div className="mt-6 flex items-center justify-between">
				<h2 className="text-base font-semibold">
					Hindsight — what the tape offered vs. what we took
				</h2>
				{backtest.captureId !== null ? (
					<button
						type="button"
						className="rounded border px-2 py-1 text-xs hover:bg-white/10"
						onClick={() => {
							publishBacktestCommand(
								"hindsight",
								undefined,
								backtest.captureId ?? undefined,
							);
						}}
					>
						Re-analyze
					</button>
				) : null}
			</div>

			{hindsight === null ? (
				<p className="mt-2 text-sm text-muted-foreground">
					Select a capture to analyze its tape in hindsight — the
					panel shows the absolute maximum the tape contained and
					which signals kept the system out of each move.
				</p>
			) : null}
			{hindsight !== null && hindsight.status === "analyzing" ? (
				<p className="mt-2 text-sm text-muted-foreground">
					Analyzing capture #{hindsight.captureId}…
				</p>
			) : null}
			{hindsight !== null && hindsight.status === "error" ? (
				<p className="mt-2 text-sm text-rose-400">
					Hindsight analysis failed for capture #{hindsight.captureId}.
				</p>
			) : null}
			{hindsight !== null && hindsight.status === "ready" ? (
				<div className="mt-2">
					<HindsightPanel report={hindsight} />
				</div>
			) : null}
		</Section>
	);
};

export const Route = createFileRoute("/backtest")({
	component: BacktestRoute,
});
