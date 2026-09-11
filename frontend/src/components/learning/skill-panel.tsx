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
				<Reading
					label="Simulated outcomes"
					value={
						skill?.defined
							? `${skill.wins} profitable · ${skill.losses} unprofitable`
							: "evaluating"
					}
					note="Simulated trade executions evaluated with venue fees and mark-to-market accounting."
				/>
				<Reading
					label="Main agent wallet P&L"
					value={
						view?.lanes?.find((lane) => lane.mode === "policy")
							? amount(
									view.lanes.find((lane) => lane.mode === "policy")?.profit ?? 0,
								)
							: "unmeasured"
					}
					note="Net profit in the Main Agent's execution wallet, including maker/taker fees."
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
				<Reading
					label="Learned situations"
					value={situations > 0 ? situations.toLocaleString() : "none yet"}
					note="Associations stored across every parallel learner. Pure directional precursor discovery."
				/>
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
