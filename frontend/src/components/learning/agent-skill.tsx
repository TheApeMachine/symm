import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { type Skill, useAgentSkill } from "./state";

/*
Tone encodes what the agent is doing and which account is exposed, not whether
that is good. Learning is the resting state and reads blue: an agent that has
not earned an edge is behaving correctly, and a distinct hue keeps it from being
read on the same red/orange/green scale the transports use. Trading is green,
and a real account additionally pulses — the one state an operator must never
mistake for any other.
*/
const tone = (mode: string) => {
	if (mode !== "trading") {
		return "info" as const;
	}

	return "success" as const;
};

const percent = (value: number) => `${(100 * value).toFixed(4)}%`;
const basis = (value: number) => `${(10000 * value).toFixed(1)} bp`;

/*
skillTitle states the whole measurement in one hover: the estimate, the bar it
has to clear, and what the agent is currently allowed to do. Every number here
is measured; none of them is a score invented for display.
*/
const skillTitle = (skill: Skill) =>
	skill.defined
		? `Mean completed decision benefit ${basis(skill.mean)} · ${skill.samples} outcomes · ${skill.wins} positive / ${skill.losses} negative. Overlapping decisions are not independent trials.`
		: "Waiting for completed tape outcomes";

export const AgentSkill = () => {
	const { state, error } = useAgentSkill();
	const skill = state?.skill;

	return (
		<Flex.Row align="center" gap={6}>
			<Badge
				label="Agent"
				variant={
					skill && !error ? tone(state?.authorizedMode ?? "learning") : "error"
				}
				dot
				pulse={state?.authorizedMode === "trading" && skill?.account === "real"}
				title={
					skill
						? `${skillTitle(skill)}\nEffective mode: ${state?.authorizedMode ?? "unavailable"} · Realization: ${state?.realizationReason ?? "unavailable"}`
						: error || "Waiting for the learning workspace"
				}
			/>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Skill
				</Typography.Label>
				<Typography.Mono
					size="lg"
					tone={skill?.defined && skill.mean > 0 ? "accent" : "f1"}
					data-agent-skill={skill?.mode ?? "offline"}
					data-agent-account={skill?.account ?? "none"}
					title={skill ? skillTitle(skill) : undefined}
				>
					{skill?.defined ? percent(skill.mean) : "—"}
				</Typography.Mono>
			</Flex.Column>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Edge
				</Typography.Label>
				<Typography.Mono
					size="lg"
					tone={!skill?.defined ? "f1" : skill.mean > 0 ? "accent" : "f2"}
					data-agent-edge={skill?.defined ? String(skill.mean) : ""}
					title="Mean completed tape evaluation, as a fraction of starting capital"
				>
					{skill?.defined ? basis(skill.mean) : "—"}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};
