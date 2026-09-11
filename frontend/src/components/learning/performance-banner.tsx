import { useSelector } from "@tanstack/react-store";
import { learningStore } from "#/collections/learning";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { amount, basis } from "./format";
import type { LearningView } from "./state";

interface PerformanceBannerProps {
	view: LearningView | null;
}

export const LearningPerformanceBanner = ({ view }: PerformanceBannerProps) => {
	const state = useSelector(learningStore, (held) => held);
	const recognition = state?.recognition;

	// Precursor Model Statistics across all parallel learners
	const learners = recognition?.learners ?? [];
	let totalAnswers = 0;
	let matchedAnswers = 0;
	let totalContrast = 0;
	let totalLinks = 0;

	for (const learner of learners) {
		totalLinks += learner.links ?? 0;
		for (const answer of learner.answers ?? []) {
			if (!answer) continue;
			totalAnswers++;
			totalContrast += answer.contrast ?? 0;
			if (
				answer.asked &&
				answer.answered &&
				answer.asked === answer.answered &&
				(answer.contrast ?? 0) > 0
			) {
				matchedAnswers++;
			}
		}
	}

	const precursorAccuracy =
		totalAnswers > 0 ? (matchedAnswers / totalAnswers) * 100 : null;
	const avgContrast =
		totalAnswers > 0 ? totalContrast / totalAnswers : null;

	// Main Agent (Agent 1) Forward Testing Performance
	const mainAgent = view?.agents?.[0];
	const skill = view?.skill;
	const samples = skill?.samples ?? 0;
	const wins = skill?.wins ?? 0;
	const losses = skill?.losses ?? 0;
	const totalRated = wins + losses;
	const winRate = totalRated > 0 ? (wins / totalRated) * 100 : null;
	const edge = skill?.defined ? skill.mean : 0;
	const realized = Number(mainAgent?.realized ?? 0);
	const unrealized = Number(mainAgent?.unrealized ?? 0);
	const profit = Number(mainAgent?.profit ?? view?.lanes?.[0]?.profit ?? 0);
	const isTrading = skill?.mode === "trading";
	const isPaused =
		totalRated >= 3 &&
		(edge === null || edge <= 0);

	// Robustness / Promotion criteria to switch to paper/real trading
	const sampleGate = samples >= 10;
	const edgeGate = edge !== null && edge > 0;
	const pnlGate = realized > 0;
	const confGate =
		Boolean(skill?.varianceDefined) &&
		edge !== null &&
		edge > 0 &&
		(skill?.variance ?? 0) < edge * edge * 4;

	const gatesPassed = [sampleGate, edgeGate, pnlGate, confGate].filter(
		Boolean,
	).length;

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
							variant={isTrading ? "success" : isPaused ? "warning" : "brand"}
							dot
							pulse={isTrading && skill?.account === "real"}
							label={
								isTrading
									? `MAIN AGENT · ${skill?.account?.toUpperCase()} TRADING`
									: isPaused
										? "MAIN AGENT · FORWARD TESTING (NEGATIVE EDGE)"
										: "MAIN AGENT · FORWARD TESTING (SIMULATED)"
							}
							title={
								isPaused
									? `Forward testing with negative edge (${basis(edge)}). Policy continues to simulate entries as precursor model trains.`
									: isTrading
										? `Live/paper trading active on ${skill?.account} account.`
										: "Main Agent executes trades based on the precursor model developed by parallel learners"
							}
						/>
					</Flex.Row>

					<Flex.Row align="center" gap={3}>
						<Typography.Mono size="s" tone="f3">
							Promotion Readiness:
						</Typography.Mono>
						<div className="flex items-center gap-1.5">
							<span
								className={`inline-block h-2 w-2 rounded-full ${
									sampleGate ? "bg-(--up)" : "bg-(--line)"
								}`}
								title={
									sampleGate
										? `Sample size gate passed (N = ${samples} ≥ 10)`
										: `Sample size: ${samples}/10 simulated trades`
								}
							/>
							<span
								className={`inline-block h-2 w-2 rounded-full ${
									edgeGate ? "bg-(--up)" : "bg-(--line)"
								}`}
								title={
									edgeGate
										? `Positive edge gate passed (${basis(edge ?? 0)})`
										: "Edge: must be > 0 bp"
								}
							/>
							<span
								className={`inline-block h-2 w-2 rounded-full ${
									pnlGate ? "bg-(--up)" : "bg-(--line)"
								}`}
								title={
									pnlGate
										? `Net profit gate passed (${amount(profit)})`
										: "Net profit: must be > $0"
								}
							/>
							<span
								className={`inline-block h-2 w-2 rounded-full ${
									confGate ? "bg-(--up)" : "bg-(--line)"
								}`}
								title={
									confGate
										? "Confidence gate passed (dispersion controlled)"
										: "Confidence: edge must exceed variance bounds"
								}
							/>
						</div>
						<Typography.Mono
							size="s"
							tone={gatesPassed === 4 ? "accent" : "f3"}
						>
							{gatesPassed}/4 criteria
						</Typography.Mono>
					</Flex.Row>
				</Flex.Row>

				{/* Two Pillar Performance Display */}
				<div className="grid grid-cols-1 gap-3 md:grid-cols-2">
					{/* Pillar 1: Precursor Model Learning Performance */}
					<div className="rounded-[3px] border border-(--line) bg-(--bg) p-3">
						<Flex.Row align="center" justify="between" className="mb-2">
							<Typography.Label size="s" tone="f2" weight="normal">
								PRECURSOR RECOGNITION (PARALLEL AGENTS)
							</Typography.Label>
							<Typography.Mono size="s" tone="f4">
								Decoupled from Economics
							</Typography.Mono>
						</Flex.Row>

						<div className="grid grid-cols-3 gap-2">
							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Learned Situations
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1">
									{(view?.decisions ?? totalLinks).toLocaleString()}
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Across {learners.length || 7} learners
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Consensus Accuracy
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone={
										precursorAccuracy !== null && precursorAccuracy > 50
											? "accent"
											: "f2"
									}
								>
									{precursorAccuracy !== null
										? `${precursorAccuracy.toFixed(1)}%`
										: "Measuring"}
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									{totalAnswers > 0
										? `${matchedAnswers}/${totalAnswers} verified`
										: "Waiting for tape"}
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Mean Contrast
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1">
									{avgContrast !== null
										? `${avgContrast.toFixed(2)} bits`
										: "—"}
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Direction separation
								</Typography.Mono>
							</Flex.Column>
						</div>
					</div>

					{/* Pillar 2: Main Agent Forward-Testing Performance */}
					<div className="rounded-[3px] border border-(--line) bg-(--bg) p-3">
						<Flex.Row align="center" justify="between" className="mb-2">
							<Typography.Label size="s" tone="f2" weight="normal">
								MAIN AGENT FORWARD TESTING (POLICY TRADER)
							</Typography.Label>
							<Badge
								variant={isTrading ? "success" : "info"}
								label={isTrading ? "PAPER / REAL" : "SIMULATED ECONOMICS"}
							/>
						</Flex.Row>

						<div className="grid grid-cols-4 gap-2">
							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Measured Edge
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone={
										edge !== null && edge > 0
											? "accent"
											: edge !== null && edge < 0
												? "f3"
												: "f1"
									}
									className={
										edge !== null && edge > 0
											? "text-(--up)"
											: edge !== null && edge < 0
												? "text-(--down)"
												: ""
									}
								>
									{edge !== null ? basis(edge) : "unmeasured"}
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									Mean trade return
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Simulated Win Rate
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									tone={
										winRate !== null && winRate >= 50
											? "accent"
											: "f1"
									}
								>
									{winRate !== null ? `${winRate.toFixed(1)}%` : "—"}
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									{wins}W · {losses}L
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Simulated Realized P&L
								</Typography.Mono>
								<Typography.Mono
									size="lg"
									className={realized >= 0 ? "text-(--up)" : "text-(--down)"}
								>
									{amount(realized)}
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									{unrealized !== 0
										? `${amount(unrealized)} open`
										: "Closed trades net"}
								</Typography.Mono>
							</Flex.Column>

							<Flex.Column className="gap-0.5">
								<Typography.Mono size="s" tone="f4">
									Trades Graded
								</Typography.Mono>
								<Typography.Mono size="lg" tone="f1">
									{samples}
								</Typography.Mono>
								<Typography.Mono size="s" tone="f4">
									{sampleGate ? "Target reached" : `Target: 10 (${10 - samples} left)`}
								</Typography.Mono>
							</Flex.Column>
						</div>
					</div>
				</div>
			</div>
		</Section>
	);
};
