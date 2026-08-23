import type {
	HindsightRecommendation,
	HindsightReport,
	HindsightRootCause,
} from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Stat } from "#/components/ui/stat";

import {
	formatHindsightCategory,
	hindsightDiagnosticCoverage,
	hindsightLegCaptureRate,
	hindsightValueCaptureRate,
	rankHindsightRecommendations,
	rankHindsightRootCauses,
} from "./hindsight-model";
import { HindsightSymbolCard } from "./hindsight-symbol";

const formatPct = (value: number): string => `${(value * 100).toFixed(2)}%`;

const RootCauseStrip = ({ causes }: { causes: HindsightRootCause[] }) =>
	causes.length === 0 ? null : (
		<div className="flex shrink-0 flex-col gap-1.5 border-b border-(--line) bg-(--sunken) px-4 py-2.5">
			<span className="font-mono text-[9px] uppercase tracking-widest text-(--f4)">
				Where value was lost
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
						<span className="font-mono text-[9px] tabular-nums text-(--warn)">
							{formatPct(cause.impactPct)}
						</span>
						<span className="font-mono text-[8px] text-(--f4)">
							{cause.occurrences} miss{cause.occurrences === 1 ? "" : "es"} · {cause.symbols.length} symbol{cause.symbols.length === 1 ? "" : "s"}
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
			Inspect each retained leg; no single stable boundary applies.
		</p>
	);

const PriorityExperiments = ({
	recommendations,
}: {
	recommendations: HindsightRecommendation[];
}) => (
	<div className="flex shrink-0 flex-col gap-2 border-b border-(--line) px-4 py-3">
		<Flex.Row align="center" justify="between">
			<div>
				<p className="font-mono text-[9px] uppercase tracking-widest text-(--acc)">
					Priority experiments
				</p>
				<p className="font-mono text-[10px] text-(--f4)">
					Ranked by associated missed value; candidates are replay tests, not live settings.
				</p>
			</div>
			<span className="font-mono text-[9px] text-(--f4)">
				{recommendations.length} causal action{recommendations.length === 1 ? "" : "s"}
			</span>
		</Flex.Row>
		{recommendations.length === 0 ? (
			<p className="rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-2 font-mono text-[10px] text-(--f4)">
				No structured recommendation arrived. Re-run the capture after retaining decision-stage outcomes.
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
							<span className="ml-auto font-mono text-[9px] tabular-nums text-(--warn)">
								{formatPct(recommendation.impactPct)}
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
	const recommendations = rankHindsightRecommendations(report);
	const rootCauses = rankHindsightRootCauses(report);
	const valueCapture = hindsightValueCaptureRate(report);
	const legCapture = hindsightLegCaptureRate(report);
	const diagnosticCoverage = hindsightDiagnosticCoverage(report);
	const missedSymbols = report.symbols.filter((symbol) => symbol.missedLegs > 0);

	return (
		<Flex.Column className="min-h-0 flex-1 overflow-hidden">
			<div className="grid shrink-0 grid-cols-4 divide-x divide-(--line) border-b border-(--line)">
				<Stat layout="tile" label="perfect-execution ceiling" value={formatPct(report.upboundPct)} className="px-4 py-3" />
				<Stat layout="tile" label="value captured" value={formatPct(valueCapture)} className="px-4 py-3" />
				<Stat layout="tile" label="missed value" value={formatPct(report.missedPct)} variant="error" className="px-4 py-3" />
				<Stat layout="tile" label="diagnosis coverage" value={formatPct(diagnosticCoverage)} className="px-4 py-3" />
			</div>

			<div className="min-h-0 flex-1 overflow-auto">
				<div className="sticky top-0 z-10 bg-(--surface)">
					<div className="flex items-center gap-4 border-b border-(--line) px-4 py-1.5 font-mono text-[9px] text-(--f4)">
						<span>legs captured {formatPct(legCapture)}</span>
						<span>{report.missedLegs} / {report.totalLegs} legs missed</span>
						<span>{missedSymbols.length} affected symbols</span>
					</div>
					<RootCauseStrip causes={rootCauses} />
					<PriorityExperiments recommendations={recommendations} />
				</div>

				{missedSymbols.length === 0 ? (
					<p className="px-4 py-8 text-center font-mono text-[10px] text-(--f4)">
						No perfect-execution legs were left unentered in this capture.
					</p>
				) : (
					<ul>
						{missedSymbols.map((symbol, index) => (
							<HindsightSymbolCard key={symbol.symbol} symbol={symbol} defaultOpen={index === 0} />
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
				? "Select a capture to see where value was lost, the exact recorded blockers, and the smallest measurable experiment that could recover similar opportunities next time."
				: "Capture loaded. Run hindsight to diagnose the first failed decision stage and rank the next parameter, measurement, allocation, or execution experiments."}
		</p>
	</Flex.Column>
);
