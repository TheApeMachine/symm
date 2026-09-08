import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { OutcomeRange } from "./charts";
import { basis } from "./format";
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
 return <Section fit="content">
  <Section.Header title="Agent skill" meta={skill ? `${skill.samples} completed decisions` : "waiting for observations"} />
  <OutcomeRange skill={skill} />
  <Reading label="Mean completed decision benefit" value={skill?.defined ? basis(skill.mean) : "unmeasured"}
   note="Benefit relative to leaving the position unchanged. Negative outcomes remain negative." />
  <Reading label="Outcome signs" value={skill ? `${skill.wins} positive · ${skill.losses} negative` : "unmeasured"}
   note="These decisions can overlap in time; the count is not independent statistical evidence." />
  <Reading label="Wallet performance" value={view?.lanes?.find(lane => lane.mode === "policy") ? String(view.lanes.find(lane => lane.mode === "policy")?.profit) : "unmeasured"}
   note="Net change in the consolidated-model agent's wallet, including fees." />
  <Reading label="Status" value={view?.status ?? "waiting"} note="All agents continue learning. No statistical promotion gate is applied." />
 </Section>;
};
