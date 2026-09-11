import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { amount, basis } from "./format";
import type { LearningView } from "./state";

const Reading = ({
	label,
	value,
	note,
	tone = "f1",
}: {
	label: string;
	value: string;
	note: string;
	tone?: "f1" | "f2" | "accent";
}) => (
	<Flex.Column className="gap-px border-(--line) border-b p-3">
		<Typography.Label size="s" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono size="lg" tone={tone}>
			{value}
		</Typography.Mono>
		<Typography.Mono size="s" tone="f4">
			{note}
		</Typography.Mono>
	</Flex.Column>
);

export const SkillPanel = ({ view }: { view: LearningView | null }) => {
	const skill = view?.skill;
	const situations = view?.decisions ?? 0;
	const hottest = view?.regions?.[0];
	const isTrading = skill?.mode === "trading";
	const policyLane = view?.lanes?.find((lane) => lane.mode === "policy");
	const profit = policyLane?.profit ?? 0;
	const totalGraded = (skill?.wins ?? 0) + (skill?.losses ?? 0);

	return (
		<>
			<Section fit="content">
				<Section.Header
					title="Main agent skill"
					meta={
						skill?.defined
							? `${skill.samples} forward evaluations · ${isTrading ? "trading" : "simulated"}`
							: "forward testing · simulated"
					}
				/>
				<Reading
					label="Mean forward trade return"
					value={skill?.defined ? basis(skill.mean) : "unmeasured"}
					note="Forward testing outcomes evaluated against the live book. Positive edge indicates readiness for paper trading."
					tone={skill?.defined && skill.mean > 0 ? "accent" : "f1"}
				/>
				<Flex.Column className="gap-1 border-(--line) border-b p-3">
					<Flex.Row justify="between" align="center">
						<Typography.Label size="s" tone="f4" weight="normal">
							Simulated outcomes
						</Typography.Label>
						{totalGraded > 0 && (
							<Typography.Mono size="s" tone="f3">
								{skill?.wins ?? 0}W · {skill?.losses ?? 0}L
							</Typography.Mono>
						)}
					</Flex.Row>
					<Typography.Mono size="lg" tone="f1">
						{skill?.defined
							? `${skill.wins} profitable · ${skill.losses} unprofitable`
							: "evaluating"}
					</Typography.Mono>
					{totalGraded > 0 ? (
						<div className="flex h-1.5 w-full overflow-hidden rounded-[2px] bg-(--line)">
							<div
								style={{
									width: `${((skill?.wins ?? 0) / totalGraded) * 100}%`,
									background: "var(--up)",
								}}
							/>
							<div
								style={{
									width: `${((skill?.losses ?? 0) / totalGraded) * 100}%`,
									background: "var(--error)",
								}}
							/>
						</div>
					) : (
						<Typography.Mono size="s" tone="f4">
							Simulated trade executions evaluated with venue fees and mark-to-market accounting.
						</Typography.Mono>
					)}
				</Flex.Column>
				<Reading
					label="Main agent wallet P&L"
					value={policyLane ? amount(profit) : "unmeasured"}
					note="Net profit in the Main Agent's execution wallet, including maker/taker fees."
					tone={profit > 0 ? "accent" : "f1"}
				/>
			</Section>

			<Section fit="content">
				<Section.Header
					title="Precursor memory"
					meta={
						situations > 0
							? `${situations.toLocaleString()} learned situations`
							: "waiting for recognition"
					}
				/>
				<Flex.Column className="gap-1 border-(--line) border-b p-3">
					<Typography.Label size="s" tone="f4" weight="normal">
						Learned situations
					</Typography.Label>
					<Typography.Mono size="lg" tone="accent" className="font-bold">
						{situations > 0 ? situations.toLocaleString() : "none yet"}
					</Typography.Mono>
					<div className="h-1.5 w-full overflow-hidden rounded-[2px] bg-(--line)">
						<div
							className="h-full bg-(--acc) transition-all duration-300"
							style={{
								width: `${Math.min(100, Math.max(5, (situations / 100000) * 100))}%`,
							}}
						/>
					</div>
					<Typography.Mono size="s" tone="f4">
						Associations stored across every parallel learner. Pure directional precursor discovery.
					</Typography.Mono>
				</Flex.Column>
				<Reading
					label="Hottest region"
					value={
						hottest
							? `#${hottest.id} · ${hottest.members} cells`
							: "none selected"
					}
					note={
						hottest
							? `${hottest.strength.toFixed(2)} energy · ${(100 * hottest.authority).toFixed(1)}% authority`
							: "The impulse map has not selected a community yet."
					}
				/>
				<Reading
					label="Status"
					value={view?.status ?? "waiting"}
					note="Co-activation across order book quantities. Recognition answers directional precursor queries."
				/>
			</Section>
		</>
	);
};
