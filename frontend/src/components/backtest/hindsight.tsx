import { useState } from "react";

import type {
	HindsightRecommendation,
	HindsightReport,
	HindsightRootCause,
	HindsightSymbol,
} from "#/collections/app";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Stat } from "#/components/ui/stat";

import {
	formatHindsightCategory,
	hindsightDiagnosticCoverage,
	hindsightLegCaptureRate,
	hindsightLossPct,
	hindsightLossPositions,
	hindsightValueCaptureRate,
	rankHindsightLossRecommendations,
	rankHindsightLossRootCauses,
	rankHindsightRecommendations,
	rankHindsightRootCauses,
} from "./hindsight-model";
import { HindsightSymbolCard } from "./hindsight-symbol";

const formatPct = (value: number): string => `${(value * 100).toFixed(2)}%`;

const hasLosses = (symbol: HindsightSymbol): boolean =>
	(symbol.lossPositions ?? 0) > 0 || (symbol.losses?.length ?? 0) > 0;

const RootCauseStrip = ({
	title,
	causes,
	variant = "warn",
}: {
	title: string;
	causes: HindsightRootCause[];
	variant?: "warn" | "error";
}) =>
	causes.length === 0 ? null : (
		<div className="flex shrink-0 flex-col gap-1.5 border-b border-(--line) bg-(--sunken) px-4 py-2.5">
			<span
				className={`font-mono text-[9px] uppercase tracking-widest ${
					variant === "error" ? "text-(--down)" : "text-(--f4)"
				}`}
			>
				{title}
			</span>
			<div className="flex flex-wrap gap-1.5">
				{causes.map((cause) => (
					<div
						key={cause.category}
						className="flex items-center gap-2 rounded-[3px] border border-(--line) bg-(--surface) px-2 py-1"
					>
						<span className="font-mono text-[9px] font-semibold text-(--f2)">
							{formatHindsightCategory(cause.category)}
						</span>
						<span
							className={`font-mono text-[9px] tabular-nums ${
								variant === "error" ? "text-(--down)" : "text-(--warn)"
							}`}
						>
							{variant === "error" ? "-" : ""}{formatPct(cause.impactPct)}
						</span>
						<span className="font-mono text-[8px] text-(--f4)">
							{cause.occurrences} instance{cause.occurrences === 1 ? "" : "s"} · {cause.symbols.length} symbol{cause.symbols.length === 1 ? "" : "s"}
						</span>
					</div>
				))}
			</div>
		</div>
	);

const BoundaryCandidate = ({
	recommendation,
}: {
	recommendation: HindsightRecommendation;
}) =>
	recommendation.hasCurrent && recommendation.hasSuggested ? (
		<p className="font-mono text-[9px] tabular-nums text-(--warn)">
			current {recommendation.current.toFixed(4)} → test {recommendation.suggested.toFixed(4)}
		</p>
	) : (
		<p className="font-mono text-[9px] text-(--f4)">
			Inspect each retained instance; no single stable boundary applies.
		</p>
	);

const PriorityExperiments = ({
	title,
	subtitle,
	recommendations,
	variant = "warn",
}: {
	title: string;
	subtitle: string;
	recommendations: HindsightRecommendation[];
	variant?: "warn" | "error";
}) => (
	<div className="flex shrink-0 flex-col gap-2 border-b border-(--line) px-4 py-3">
		<Flex.Row align="center" justify="between">
			<div>
				<p
					className={`font-mono text-[9px] uppercase tracking-widest ${
						variant === "error" ? "text-(--down)" : "text-(--acc)"
					}`}
				>
					{title}
				</p>
				<p className="font-mono text-[10px] text-(--f4)">{subtitle}</p>
			</div>
			<span className="font-mono text-[9px] text-(--f4)">
				{recommendations.length} causal action{recommendations.length === 1 ? "" : "s"}
			</span>
		</Flex.Row>
		{recommendations.length === 0 ? (
			<p className="rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-2 font-mono text-[10px] text-(--f4)">
				No structured recommendations arrived for this category.
			</p>
		) : (
			<div className="grid grid-cols-2 gap-2 xl:grid-cols-3">
				{recommendations.slice(0, 6).map((recommendation, index) => (
					<div
						key={recommendation.key}
						className="flex min-w-0 flex-col gap-1 rounded-[3px] border border-(--line) bg-(--sunken) p-2.5"
					>
						<Flex.Row align="center" gap={2}>
							<span className="font-mono text-[9px] text-(--f4)">#{index + 1}</span>
							<span className="truncate font-mono text-[10px] font-semibold text-(--f1)">
								{recommendation.title}
							</span>
							<span
								className={`ml-auto font-mono text-[9px] tabular-nums ${
									variant === "error" ? "text-(--down)" : "text-(--warn)"
								}`}
							>
								{variant === "error" ? "-" : ""}{formatPct(recommendation.impactPct)}
							</span>
						</Flex.Row>
						<p className="truncate font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							{recommendation.kind.replaceAll("_", " ")} · {recommendation.target}
						</p>
						<BoundaryCandidate recommendation={recommendation} />
						<p className="line-clamp-3 font-mono text-[9px] leading-relaxed text-(--f3)">
							{recommendation.action}
						</p>
						<p className="font-mono text-[8px] text-(--f4)">
							{recommendation.occurrences} occurrence{recommendation.occurrences === 1 ? "" : "s"} · {(recommendation.confidence * 100).toFixed(0)}% evidence · {recommendation.symbols.join(", ")}
						</p>
					</div>
				))}
			</div>
		)}
	</div>
);

export const HindsightPanel = ({ report }: { report: HindsightReport }) => {
	const [activeTab, setActiveTab] = useState<"all" | "opportunities" | "losses">("all");

	const recommendations = rankHindsightRecommendations(report);
	const rootCauses = rankHindsightRootCauses(report);
	const lossRecommendations = rankHindsightLossRecommendations(report);
	const lossRootCauses = rankHindsightLossRootCauses(report);

	const valueCapture = hindsightValueCaptureRate(report);
	const legCapture = hindsightLegCaptureRate(report);
	const diagnosticCoverage = hindsightDiagnosticCoverage(report);
	const lossPct = hindsightLossPct(report);
	const lossPositions = hindsightLossPositions(report);

	const relevantSymbols = report.symbols.filter((symbol) => {
		if (activeTab === "opportunities") {
			return symbol.missedLegs > 0;
		}

		if (activeTab === "losses") {
			return hasLosses(symbol);
		}

		return symbol.missedLegs > 0 || hasLosses(symbol);
	});

	return (
		<Flex.Column className="min-h-0 flex-1 overflow-hidden">
			<div className="grid shrink-0 grid-cols-5 divide-x divide-(--line) border-b border-(--line)">
				<Stat layout="tile" label="perfect-execution ceiling" value={formatPct(report.upboundPct)} className="px-4 py-3" />
				<Stat layout="tile" label="value captured" value={formatPct(valueCapture)} className="px-4 py-3" />
				<Stat layout="tile" label="missed value" value={formatPct(report.missedPct)} variant="error" className="px-4 py-3" />
				<Stat
					layout="tile"
					label="realized loss"
					value={lossPct > 0 ? `-${formatPct(lossPct)} (${lossPositions})` : "0.00%"}
					variant={lossPct > 0 ? "error" : "default"}
					className="px-4 py-3"
				/>
				<Stat layout="tile" label="diagnosis coverage" value={formatPct(diagnosticCoverage)} className="px-4 py-3" />
			</div>

			<div className="min-h-0 flex-1 overflow-auto">
				<div className="sticky top-0 z-10 bg-(--surface)">
					<div className="flex items-center justify-between border-b border-(--line) px-4 py-1.5 font-mono text-[9px] text-(--f4)">
						<div className="flex items-center gap-4">
							<span>legs captured {formatPct(legCapture)}</span>
							<span>{report.missedLegs} / {report.totalLegs} legs missed</span>
							{lossPositions > 0 ? (
								<span className="text-(--down)">{lossPositions} loss positions</span>
							) : null}
							<span>{relevantSymbols.length} displayed symbols</span>
						</div>
						<div className="flex items-center gap-1">
							<Button
								variant={activeTab === "all" ? "default" : "bare"}
								size="sm"
								className="h-5 px-2 font-mono text-[9px]"
								aria-pressed={activeTab === "all"}
								onClick={() => setActiveTab("all")}
							>
								All
							</Button>
							<Button
								variant={activeTab === "opportunities" ? "default" : "bare"}
								size="sm"
								className="h-5 px-2 font-mono text-[9px]"
								aria-pressed={activeTab === "opportunities"}
								onClick={() => setActiveTab("opportunities")}
							>
								Missed Opportunities ({report.missedLegs})
							</Button>
							<Button
								variant={activeTab === "losses" ? "default" : "bare"}
								size="sm"
								className="h-5 px-2 font-mono text-[9px]"
								aria-pressed={activeTab === "losses"}
								onClick={() => setActiveTab("losses")}
							>
								Trade Post-Mortem ({lossPositions})
							</Button>
						</div>
					</div>

					{(activeTab === "all" || activeTab === "losses") && lossRootCauses.length > 0 ? (
						<RootCauseStrip
							title="Where capital was lost (loss post-mortem)"
							causes={lossRootCauses}
							variant="error"
						/>
					) : null}

					{(activeTab === "all" || activeTab === "opportunities") && rootCauses.length > 0 ? (
						<RootCauseStrip
							title="Where value was missed (untapped moves)"
							causes={rootCauses}
							variant="warn"
						/>
					) : null}

					{(activeTab === "all" || activeTab === "losses") && lossRecommendations.length > 0 ? (
						<PriorityExperiments
							title="Priority risk & execution fixes"
							subtitle="Ranked by realized position loss; mitigations for adverse selection, friction drag, and whipsaw stopouts."
							recommendations={lossRecommendations}
							variant="error"
						/>
					) : null}

					{(activeTab === "all" || activeTab === "opportunities") && recommendations.length > 0 ? (
						<PriorityExperiments
							title="Priority opportunity experiments"
							subtitle="Ranked by associated missed value; candidates are replay tests, not live settings."
							recommendations={recommendations}
							variant="warn"
						/>
					) : null}
				</div>

				{relevantSymbols.length === 0 ? (
					<p className="px-4 py-8 text-center font-mono text-[10px] text-(--f4)">
						No diagnostic items found for the current view.
					</p>
				) : (
					<ul>
						{relevantSymbols.map((symbol, index) => (
							<HindsightSymbolCard
								key={symbol.symbol}
								symbol={symbol}
								defaultOpen={index === 0}
								activeTab={activeTab}
							/>
						))}
					</ul>
				)}
			</div>
		</Flex.Column>
	);
};

export const HindsightEmpty = ({ captureId }: { captureId: number | null }) => (
	<Flex.Column align="center" justify="center" gap={3} className="flex-1 p-8 text-center">
		<span className="font-mono text-[10px] uppercase tracking-widest text-(--f4)">Hindsight</span>
		<p className="max-w-md font-mono text-[11px] leading-relaxed text-(--f3)">
			{captureId === null || captureId === 0
				? "Select a capture to see where value was lost, why losing positions entered, the exact recorded blockers, and the smallest measurable experiments that could recover performance next time."
				: "Capture loaded. Run hindsight to diagnose missed opportunities and post-mortem losing trades with honest market friction and microstructure depth."}
		</p>
	</Flex.Column>
);
