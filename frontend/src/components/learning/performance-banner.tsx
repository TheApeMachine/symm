import React from "react";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";

export const LearningPerformanceBanner = React.memo(() => {
	return (
		<Section
			fit="content"
			className="shrink-0 border-(--line) border-b bg-(--surface)"
		>
			<div className="flex flex-col gap-3 p-3 font-mono text-xs">
				{/* Top Status & Staged Architecture Ribbon */}
				<Flex.Row align="center" className="flex-wrap justify-between gap-3">
					<Flex.Row align="center" gap={2}>
						<Typography.Label size="s" tone="f4" weight="normal">
							SYSTEM ARCHITECTURE
						</Typography.Label>
						<Badge
							variant="info"
							label="PRECURSOR COGNITION MODEL"
							title="Single predictive radix trie learning temporal precursor fragments"
						/>
						<Typography.Mono size="s" tone="f4">
							→
						</Typography.Mono>
						<Badge
							variant="brand"
							dot
							label="STAGED LEARNING"
							title="Sequential progression: Model Development -> Historical Validation -> Forward Paper -> Forward Skill"
						/>
					</Flex.Row>

					<Flex.Row align="center" gap={3}>
						<Typography.Mono size="s" tone="f4">
							STAGE:
						</Typography.Mono>
						<span
							data-l="training-stage"
							className="px-2 py-0.5 rounded text-[10px] font-bold bg-(--acc)/10 text-(--acc) border border-(--acc)/30 uppercase tracking-wider"
						>
							MODEL DEVELOPMENT
						</span>
						<Typography.Mono size="s" tone="f4">
							BLOCKER:
						</Typography.Mono>
						<Typography.Mono
							size="s"
							tone="f3"
							data-l="stage-blocker"
							className="text-[10px] text-(--f3) max-w-xs truncate"
						>
							—
						</Typography.Mono>
						<Typography.Mono
							size="s"
							tone="f3"
							data-l="gate-count"
							className="text-[10px]"
						>
							Stage 0 · Replay
						</Typography.Mono>
					</Flex.Row>
				</Flex.Row>

				{/* Two Pillar Performance Display */}
				<div className="grid grid-cols-1 gap-3 md:grid-cols-2">
					{/* Pillar 1: Precursor Cognition & Replay Development */}
					<div className="rounded-[3px] border border-(--line) bg-(--bg) p-3 flex flex-col gap-2.5">
						<Flex.Row align="center" justify="between">
							<Typography.Label size="s" tone="f2" weight="normal">
								PRECURSOR COGNITION (MODEL DEVELOPMENT)
							</Typography.Label>
							<Typography.Mono size="s" tone="f4">
								Predictive Radix Trie
							</Typography.Mono>
						</Flex.Row>

						<div className="grid grid-cols-3 gap-2">
							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Learned Situations
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1" data-metric="decisions" data-format="integer">
									0
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									<span data-metric="steps" data-format="integer">0</span> frames
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Confidence & Ambiguity
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone="accent"
									data-metric="confidence"
									data-format="percent"
								>
									0.0%
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4" data-metric="ambiguity" data-format="spread">
									Spread 0.000
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Contrast & Precursor
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1" data-metric="contrast" data-format="bits">
									0.00 bits
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									len: <span data-metric="precursor_length" data-format="integer">0</span> tokens
								</Typography.Mono>
							</Flex.Column>
						</div>

						{/* Fragment Classification Distribution */}
						<div className="pt-2 border-t border-(--line) flex items-center justify-between text-[10px]">
							<span className="text-(--f4) uppercase tracking-wider">Tape Fragments:</span>
							<div className="flex items-center gap-2">
								<span className="text-(--up)">UP: <span data-metric="fragments_up" data-format="integer">0</span></span>
								<span className="text-(--up) opacity-70">UP_F: <span data-metric="fragments_up_friction" data-format="integer">0</span></span>
								<span className="text-(--down)">DN: <span data-metric="fragments_down" data-format="integer">0</span></span>
								<span className="text-(--f3)">CHOP: <span data-metric="fragments_chop" data-format="integer">0</span></span>
								<span className="text-(--f4)">FLAT: <span data-metric="fragments_flat" data-format="integer">0</span></span>
								<span className="text-(--f4)">UNSUP: <span data-metric="fragments_unsupported" data-format="integer">0</span></span>
							</div>
						</div>
					</div>

					{/* Pillar 2: Held-Out Skill and Forward Paper Evidence */}
					<div className="rounded-[3px] border border-(--line) bg-(--bg) p-3 flex flex-col gap-2.5">
						<Flex.Row align="center" justify="between">
							<Typography.Label size="s" tone="f2" weight="normal">
								HELD-OUT SKILL AND FORWARD PAPER
							</Typography.Label>
							<Badge
								variant="info"
								label="PREQUENTIAL"
								title="Predictions frozen before outcome observation"
							/>
						</Flex.Row>

						<div className="grid grid-cols-4 gap-2">
							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Hist Mean Return
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone="accent"
									data-metric="hist_mean_return"
									data-format="insufficient_if_zero"
								>
									—
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Lower: <span data-metric="hist_lower_bound" data-format="insufficient_if_zero">—</span>
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Hist Recognition
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1">
									<span data-metric="hist_correct_enter" data-format="integer">0</span>
									<span className="text-xs text-(--f4)">/<span data-metric="hist_opportunities" data-format="integer">0</span></span>
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									False: <span data-metric="hist_false_enter" data-format="integer">0</span>
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Fwd Paper Return
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone="accent"
									data-metric="fwd_paper_mean_return"
									data-format="insufficient_if_zero"
								>
									—
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Lower: <span data-metric="fwd_paper_lower_bound" data-format="insufficient_if_zero">—</span>
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Fwd Paper Trades
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1" data-metric="fwd_paper_trades" data-format="integer">
									0
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Pred: <span data-metric="fwd_enter_predictions" data-format="integer">0</span>
								</Typography.Mono>
							</Flex.Column>
						</div>

						{/* Held-Out Exit & Forward Recognition Details */}
						<div className="pt-2 border-t border-(--line) flex items-center justify-between text-[10px]">
							<span className="text-(--f4) uppercase tracking-wider">Held-Out Exits:</span>
							<div className="flex items-center gap-2">
								<span className="text-(--up)">OK: <span data-metric="hist_correct_exit" data-format="integer">0</span></span>
								<span className="text-(--down)">PRE: <span data-metric="hist_premature_exit" data-format="integer">0</span></span>
								<span className="text-(--f3)">MISS: <span data-metric="hist_missed_exit" data-format="integer">0</span></span>
								<span className="text-(--f4)">WAIT: <span data-metric="hist_correct_wait" data-format="integer">0</span></span>
							</div>
						</div>
					</div>
				</div>
			</div>
		</Section>
	);
});
