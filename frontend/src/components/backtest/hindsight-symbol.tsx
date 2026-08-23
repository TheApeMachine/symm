import { useState } from "react";

import type {
	HindsightBlocker,
	HindsightOpportunity,
	HindsightRecommendation,
	HindsightSignal,
	HindsightSymbol,
} from "#/collections/app";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";

import { formatHindsightCategory } from "./hindsight-model";

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

const AlternativeBars = ({
	alternatives,
}: {
	alternatives: Record<string, number>;
}) => {
	const entries = Object.entries(alternatives).sort(
		([left], [right]) => left.localeCompare(right),
	);
	const ceiling = Math.max(
		1e-9,
		...entries.map(([, score]) => Math.abs(score)),
	);

	return (
		<div className="flex flex-col gap-1 border-t border-(--line) pt-2">
			<span className="font-mono text-[9px] uppercase tracking-widest text-(--f4)">
				recorded decision inputs
			</span>
			<ul className="flex flex-col gap-0.5">
				{entries.map(([source, score]) => {
					const clamp = Math.min(1, Math.abs(score) / ceiling);

					return (
						<li
							key={source}
							className="flex items-center gap-2 font-mono text-[10px]"
						>
							<span className="w-52 truncate text-(--f4)">{source}</span>
							<span
								className="h-1 shrink-0 rounded-full bg-(--acc) opacity-40"
								style={{ width: `${Math.max(3, clamp * 80)}px` }}
							/>
							<span className="tabular-nums text-(--f3)">{score.toFixed(4)}</span>
						</li>
					);
				})}
			</ul>
		</div>
	);
};

const ThesisRow = ({ label, value }: { label: string; value?: number }) =>
	value === undefined ? null : (
		<div className="flex items-center gap-2 font-mono text-[10px]">
			<span className="w-40 text-(--f4)">{label}</span>
			<span className="tabular-nums text-(--f3)">{value.toFixed(4)}</span>
		</div>
	);

const JournalRows = ({ journal }: { journal?: HindsightSignal[] }) =>
	journal === undefined || journal.length === 0 ? null : (
		<div className="flex flex-col gap-1 border-t border-(--line) pt-2">
			<span className="font-mono text-[9px] uppercase tracking-widest text-(--f4)">
				decision path through the missed hold
			</span>
			{journal.map((decision, index) => (
				<div
					key={`${decision.at}-${index}`}
					className="grid grid-cols-[5rem_4rem_6rem_6rem_minmax(0,1fr)] items-center gap-2 rounded-[3px] bg-(--sunken) px-2 py-1 font-mono text-[10px]"
				>
					<span className="tabular-nums text-(--f4)">{formatClock(decision.at)}</span>
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
					<span className="truncate text-(--f4)">{decision.reason}</span>
				</div>
			))}
		</div>
	);

const BlockerRows = ({ blockers }: { blockers: HindsightBlocker[] }) => (
	<div className="flex flex-col gap-1.5">
		<span className="font-mono text-[9px] uppercase tracking-widest text-(--f4)">
			causal blockers — first failure ranked first
		</span>
		{blockers.map((blocker, index) => (
			<div
				key={`${blocker.key}-${index}`}
				className="grid grid-cols-[1.25rem_minmax(10rem,0.7fr)_minmax(0,1fr)] gap-2 rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5"
			>
				<span className="font-mono text-[9px] text-(--f4)">{index + 1}</span>
				<div className="min-w-0">
					<p className="truncate font-mono text-[10px] font-semibold text-(--f2)">
						{blocker.label}
					</p>
					<p className="truncate font-mono text-[9px] text-(--f4)">
						{blocker.source || blocker.key}
					</p>
					{blocker.hasTarget ? (
						<p className="font-mono text-[9px] tabular-nums text-(--warn)">
							observed {blocker.observed.toFixed(4)} · required {blocker.target.toFixed(4)} · gap {blocker.gap.toFixed(4)}
						</p>
					) : null}
				</div>
				<p className="font-mono text-[10px] leading-relaxed text-(--f3)">
					{blocker.explanation}
				</p>
			</div>
		))}
	</div>
);

const RecommendationCard = ({
	recommendation,
	evidenceQuality,
	evidenceStatus,
}: {
	recommendation: HindsightRecommendation;
	evidenceQuality: number;
	evidenceStatus: string;
}) => (
	<div className="flex flex-col gap-1.5 rounded-[3px] border border-(--acc) bg-(--sunken) p-2.5">
		<Flex.Row align="center" gap={2}>
			<span className="font-mono text-[9px] uppercase tracking-widest text-(--acc)">
				next experiment
			</span>
			<span className="rounded-[3px] border border-(--line) px-1.5 py-px font-mono text-[8px] text-(--f4)">
				{recommendation.kind.replaceAll("_", " ")}
			</span>
			<span className="ml-auto font-mono text-[9px] tabular-nums text-(--f4)">
				evidence {evidenceStatus} · {(evidenceQuality * 100).toFixed(0)}%
			</span>
		</Flex.Row>
		<p className="font-mono text-[11px] font-semibold text-(--f1)">
			{recommendation.title}
		</p>
		<p className="font-mono text-[9px] text-(--f4)">{recommendation.target}</p>
		{recommendation.hasCurrent && recommendation.hasSuggested ? (
			<p className="font-mono text-[10px] tabular-nums text-(--warn)">
				current {recommendation.current.toFixed(4)} → counterfactual candidate {recommendation.suggested.toFixed(4)}
			</p>
		) : null}
		<p className="font-mono text-[10px] leading-relaxed text-(--f2)">
			{recommendation.action}
		</p>
		<p className="font-mono text-[9px] leading-relaxed text-(--f4)">
			Why this experiment: {recommendation.rationale}
		</p>
	</div>
);

const OpportunityCard = ({ opportunity }: { opportunity: HindsightOpportunity }) => {
	const diagnosis = opportunity.diagnosis;

	return (
		<div className="flex flex-col gap-2 rounded-[3px] border border-(--line) bg-(--surface) p-3">
			<Flex.Row align="center" gap={2} className="font-mono text-[10px]">
				<span className="text-(--up)">↑</span>
				<span className="tabular-nums text-(--f2)">
					{opportunity.leg.buyPrice} → {opportunity.leg.sellPrice}
				</span>
				<span className="tabular-nums text-(--warn)">
					{formatPct(opportunity.leg.profitPct)} missed
				</span>
				{diagnosis?.category ? (
					<span className="rounded-[3px] border border-(--warn) px-1.5 py-px text-[8px] uppercase tracking-widest text-(--warn)">
						{formatHindsightCategory(diagnosis.category)}
					</span>
				) : null}
				<span className="ml-auto text-(--f4)">
					{formatClock(opportunity.leg.buyAt)} → {formatClock(opportunity.leg.sellAt)}
				</span>
			</Flex.Row>

			<p className="rounded-[3px] border border-(--warn) bg-(--sunken) px-2 py-1.5 font-mono text-[10px] leading-relaxed text-(--warn)">
				{diagnosis?.summary || opportunity.why || "No retained diagnosis."}
			</p>

			{diagnosis !== undefined && diagnosis !== null ? (
				<>
					<BlockerRows blockers={diagnosis.blockers} />
					{diagnosis.recommendation !== null ? (
						<RecommendationCard
							recommendation={diagnosis.recommendation}
							evidenceQuality={diagnosis.evidenceQuality}
							evidenceStatus={diagnosis.evidenceStatus}
						/>
					) : null}
				</>
			) : null}

			<div className="grid grid-cols-2 gap-x-4 gap-y-0.5 border-t border-(--line) pt-2">
				<ThesisRow label="thesis confidence" value={opportunity.signal.thesisConfidence} />
				<ThesisRow label="thesis support" value={opportunity.signal.thesisSupport} />
				<ThesisRow label="thesis contradiction" value={opportunity.signal.thesisContradiction} />
				<ThesisRow label="thesis conditions" value={opportunity.signal.thesisConditions} />
				<ThesisRow label="direction" value={opportunity.signal.direction} />
				<ThesisRow label="admission threshold" value={opportunity.signal.admissionThreshold} />
			</div>
			<JournalRows journal={opportunity.journal} />
			{opportunity.signal.alternatives !== null &&
			Object.keys(opportunity.signal.alternatives).length > 0 ? (
				<AlternativeBars alternatives={opportunity.signal.alternatives} />
			) : null}
		</div>
	);
};

export const HindsightSymbolCard = ({
	symbol,
	defaultOpen = false,
}: {
	symbol: HindsightSymbol;
	defaultOpen?: boolean;
}) => {
	const [open, setOpen] = useState(defaultOpen);

	return (
		<li className="border-b border-(--line) last:border-b-0">
			<Button
				variant="bare"
				className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-(--raised)"
				onClick={() => setOpen((previous) => !previous)}
			>
				<span className="w-32 truncate font-mono text-[11px] font-semibold text-(--f1)">{symbol.symbol}</span>
				<span className="w-24 font-mono text-[10px] tabular-nums text-(--f3)">ceiling {formatPct(symbol.upboundPct)}</span>
				<span className="w-28 font-mono text-[10px] tabular-nums text-(--up)">entered {formatPct(symbol.realizedPct)}</span>
				<span className="w-28 font-mono text-[10px] tabular-nums text-(--down)">missed {formatPct(symbol.missedPct)}</span>
				<span className="font-mono text-[10px] text-(--f4)">{symbol.missedLegs}/{symbol.legs} legs</span>
				<span className="ml-auto font-mono text-[9px] text-(--f4)">{open ? "▲" : "▼"}</span>
			</Button>

			{open ? (
				<div className="flex flex-col gap-2 border-t border-(--line) bg-(--sunken) px-4 py-3">
					{symbol.opportunities.length === 0 ? (
						<p className="font-mono text-[10px] text-(--f4)">No missed legs with retained decision context.</p>
					) : null}
					{symbol.opportunities.slice(0, 8).map((opportunity) => (
						<OpportunityCard
							key={`${opportunity.leg.buyAt}-${opportunity.leg.sellAt}-${opportunity.leg.buyPrice}`}
							opportunity={opportunity}
						/>
					))}
				</div>
			) : null}
		</li>
	);
};
