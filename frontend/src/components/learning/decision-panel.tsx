import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";

export const ForwardPanel = () => (
	<Section fit="content">
		<Section.Header
			title="Forward evaluation"
			meta={<span data-l="forward-meta">0 completed evaluations</span>}
		/>
		<Section.Body className="p-3 font-mono text-[11px] flex flex-col gap-3">
			<Grid cols={4} gap={2} responsive={false}>
				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Evaluated Decisions
					</span>
					<span
						className="text-base font-bold text-(--f1)"
						data-metric="resolved"
						data-format="integer"
					>
						0
					</span>
					<span className="text-[9px] text-(--f4)">Verified causal moments</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Forward Win Rate
					</span>
					<span
						className="text-base font-bold text-(--up)"
						data-metric="win_rate"
						data-format="percent"
					>
						—
					</span>
					<span className="text-[9px] text-(--f4)">Post-friction profitability</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Mean Forward Edge
					</span>
					<span
						className="text-base font-bold text-(--acc)"
						data-metric="edge"
						data-format="basis"
					>
						—
					</span>
					<span className="text-[9px] text-(--f4)">Realized quote return</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Prediction Accuracy
					</span>
					<span
						className="text-base font-bold text-(--f1)"
						data-metric="accuracy"
						data-format="percent"
					>
						0.0%
					</span>
					<span className="text-[9px] text-(--f4)">Categorical label match</span>
				</div>
			</Grid>

			<Typography.Mono tone="f4" size="s">
				Continuous causal evaluation streams verified predictions against honest market outcomes.
			</Typography.Mono>
		</Section.Body>
	</Section>
);

export const ImpulsePanel = () => (
	<Section fit="content">
		<Section.Header
			title="Current impulse & precursor metrics"
			meta={<span data-l="impulse-meta">causal precursor active</span>}
		/>
		<Section.Body className="p-3 font-mono text-[11px]">
			<Grid cols={2} gap={2} responsive={false}>
				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1.5">
					<div className="flex justify-between items-center text-[10px]">
						<span className="text-(--f4) uppercase tracking-wider">
							Market Spread Ambiguity
						</span>
						<span className="w-1.5 h-1.5 rounded-full bg-(--acc) animate-pulse" />
					</div>
					<span
						className="text-sm font-bold text-(--f1)"
						data-metric="ambiguity"
						data-format="spread"
					>
						Spread 0.000
					</span>
					<div className="h-1.5 w-full rounded-full bg-(--sunken) border border-(--line) overflow-hidden mt-0.5">
						<div className="h-full bg-(--acc) w-1/3" />
					</div>
					<span className="text-[9px] text-(--f4)">
						Order book dispersion & frictional spread deformation
					</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1.5">
					<div className="flex justify-between items-center text-[10px]">
						<span className="text-(--f4) uppercase tracking-wider">
							Precursor Surprisal
						</span>
						<span className="w-1.5 h-1.5 rounded-full bg-(--info)" />
					</div>
					<span
						className="text-sm font-bold text-(--info)"
						data-metric="surprisal"
						data-format="surprisal"
					>
						Surprisal 0.00 nat
					</span>
					<div className="h-1.5 w-full rounded-full bg-(--sunken) border border-(--line) overflow-hidden mt-0.5">
						<div className="h-full bg-(--info) w-1/4" />
					</div>
					<span className="text-[9px] text-(--f4)">
						Information-theoretic surprise of current tape arrival
					</span>
				</div>
			</Grid>
		</Section.Body>
	</Section>
);

export const CandidatePanel = () => (
	<Section fit="content">
		<Section.Header
			title="Model action evaluation"
			meta="Causal policy candidates"
		/>
		<Section.Body className="p-3 font-mono text-[11px] flex flex-col gap-2">
			<Grid cols={4} gap={2} responsive={false}>
				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1.5">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Policy Decision
					</span>
					<span
						className="px-2 py-1 rounded text-xs font-bold text-center border border-(--line) bg-(--surface) text-(--acc)"
						data-metric="action"
						data-format="action"
					>
						ABSTAIN
					</span>
					<span className="text-[9px] text-(--f4)">Autonomous policy output</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Confidence
					</span>
					<span
						className="text-base font-bold text-(--up)"
						data-metric="confidence"
						data-format="percent"
					>
						0.0%
					</span>
					<div className="h-1.5 w-full rounded-full bg-(--sunken) border border-(--line) overflow-hidden">
						<div className="h-full bg-(--up) w-0" />
					</div>
					<span className="text-[9px] text-(--f4)">Trie path posterior belief</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Contrast
					</span>
					<span
						className="text-base font-bold text-(--f1)"
						data-metric="contrast"
						data-format="bits"
					>
						0.00 bits
					</span>
					<span className="text-[9px] text-(--f4)">Information discrimination</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Expected Edge
					</span>
					<span
						className="text-base font-bold text-(--acc)"
						data-metric="edge"
						data-format="basis"
					>
						—
					</span>
					<span className="text-[9px] text-(--f4)">Net of fees & slippage</span>
				</div>
			</Grid>
		</Section.Body>
	</Section>
);

export const InfluencePanel = () => (
	<Section fit="content">
		<Section.Header
			title="Precursor discovery & associations"
			meta="Evidence from tape cognition"
		/>
		<Section.Body className="p-3 font-mono text-[11px] flex flex-col gap-3">
			<Grid cols={3} gap={2} responsive={false}>
				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Information Contrast
					</span>
					<span
						className="text-sm font-bold text-(--f1)"
						data-metric="contrast"
						data-format="bits"
					>
						0.00 bits
					</span>
					<span className="text-[9px] text-(--f4)">Precursor partition fidelity</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Event Surprisal
					</span>
					<span
						className="text-sm font-bold text-(--info)"
						data-metric="surprisal"
						data-format="surprisal"
					>
						Surprisal 0.00 nat
					</span>
					<span className="text-[9px] text-(--f4)">Shock arrival magnitude</span>
				</div>

				<div className="p-2.5 rounded bg-(--bg) border border-(--line) flex flex-col gap-1">
					<span className="text-[9px] uppercase tracking-wider text-(--f4)">
						Ambiguity Spread
					</span>
					<span
						className="text-sm font-bold text-(--f1)"
						data-metric="ambiguity"
						data-format="spread"
					>
						Spread 0.000
					</span>
					<span className="text-[9px] text-(--f4)">Order flow uncertainty</span>
				</div>
			</Grid>

			<Typography.Mono tone="f4" size="s">
				Direct precursor associations formed across parallel cognitive learners.
			</Typography.Mono>
		</Section.Body>
	</Section>
);
