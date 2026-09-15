import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";

export const LearningPerformanceBanner = () => {
	return (
		<Section
			fit="content"
			className="shrink-0 border-(--line) border-b bg-(--surface)"
		>
			<div className="flex flex-col gap-3 p-3">
				{/* Top Status & Architecture Ribbon */}
				<Flex.Row align="center" className="flex-wrap justify-between gap-3">
					<Flex.Row align="center" gap={2}>
						<Typography.Label size="s" tone="f4" weight="normal">
							SYSTEM ARCHITECTURE
						</Typography.Label>
						<Badge
							variant="info"
							label="PARALLEL LEARNERS (DECOUPLED)"
							title="Parallel agents learn upward, downward, and stagnant movement precursors without economics"
						/>
						<Typography.Mono size="s" tone="f4">
							→
						</Typography.Mono>
						<Badge
							variant="brand"
							dot
							label="FORWARD TESTING"
						/>
					</Flex.Row>

					<Flex.Row align="center" gap={3}>
						<Typography.Mono size="s" tone="f3">
							Execution Readiness:
						</Typography.Mono>
						<Typography.Mono
							size="s"
							tone="f3"
							data-l="gate-count"
						>
							0/4 criteria
						</Typography.Mono>
					</Flex.Row>
				</Flex.Row>

				{/* Two Pillar Performance Display */}
				<div className="grid grid-cols-1 gap-3 md:grid-cols-2">
					{/* Pillar 1: Precursor Cognition Model */}
					<div className="rounded-[3px] border border-(--line) bg-(--bg) p-3">
						<Flex.Row align="center" justify="between" className="mb-2">
							<Typography.Label size="s" tone="f2" weight="normal">
								PRECURSOR COGNITION
							</Typography.Label>
							<Typography.Mono size="s" tone="f4">
								Associative Memory
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
								<Typography.Mono size="s" tone="f4" data-metric="steps" data-format="integer">
									0 frames
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Confidence
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
									Contrast
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1" data-metric="contrast" data-format="bits">
									0.00 bits
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4" data-metric="surprisal" data-format="surprisal">
									Surprisal 0.00 nat
								</Typography.Mono>
							</Flex.Column>
						</div>
					</div>

					{/* Pillar 2: Market Forward Evaluation */}
					<div className="rounded-[3px] border border-(--line) bg-(--bg) p-3">
						<Flex.Row align="center" justify="between" className="mb-2">
							<Typography.Label size="s" tone="f2" weight="normal">
								FORWARD EVALUATION
							</Typography.Label>
							<Badge
								variant="info"
								label="MEASURING"
							/>
						</Flex.Row>

						<div className="grid grid-cols-4 gap-2">
							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Measured Edge
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone="accent"
									data-metric="edge"
									data-format="basis"
								>
									0.0 bp
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Mean return
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Win Rate
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone="f1"
									data-metric="win_rate"
									data-format="percent"
								>
									0.0%
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Outcomes
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Accuracy
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone="f1"
									data-metric="accuracy"
									data-format="percent"
								>
									0.0%
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Causal prediction
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Evaluations
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1" data-metric="resolved" data-format="integer">
									0
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Target reached
								</Typography.Mono>
							</Flex.Column>
						</div>
					</div>
				</div>
			</div>
		</Section>
	);
};
