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

const percent = (value: number) => `${(100 * value).toFixed(1)}%`;
const basis = (value: number) => `${(10000 * value).toFixed(1)} bp`;

/*
skillTitle states the whole measurement in one hover: the estimate, the bar it
has to clear, and what the agent is currently allowed to do. Every number here
is measured; none of them is a score invented for display.
*/
const skillTitle = (skill: Skill) => {
	if (!skill.defined) {
		return `No resolved policy outcomes yet · ${skill.reason}`;
	}

	const dispersion = skill.qualified
		? `lower bound ${basis(skill.lowerBound)} at ${skill.sigma}σ · standard error ${basis(skill.standardError)}`
		: "evidence is below the confidence floor — no bound is stated";

	return [
		`Mean forward return per policy decision ${basis(skill.mean)}`,
		dispersion,
		`${skill.samples} disjoint windows admitted · ${skill.support.toFixed(1)} effective · ${skill.memory} window memory`,
		`${skill.wins} positive / ${skill.losses} negative`,
		`${skill.mode === "trading" ? `Skill permits increases on the ${skill.account} account` : `Calibrating — would trade the ${skill.account} account`} · went live ${skill.promotions} times, fell back ${skill.demotions}`,
		skill.reason,
	].join("\n");
};

/*
AgentSkill is the terminal's answer to "how good is it, and what is it allowed
to do". Confidence is the empirical probability that the measured edge is
positive — a statement about these observations, not a calibrated forecast.

Both readings stay blank until the evidence clears the same floor promotion
uses. A bound computed from fewer effective observations than its own
confidence multiple assumes is arithmetic, not a measurement, and putting it
on screen invites reading a saturated number as certainty.
*/
export const AgentSkill = () => {
	const { state, error } = useAgentSkill();
	const skill = state?.skill;

	return (
		<Flex.Row align="center" gap={6}>
			<Badge
				label="Agent"
				variant={skill ? tone(state?.authorizedMode ?? "learning") : "error"}
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
					tone={skill?.qualified && skill.lowerBound > 0 ? "accent" : "f1"}
					data-agent-skill={skill?.mode ?? "offline"}
					data-agent-account={skill?.account ?? "none"}
					title={skill ? skillTitle(skill) : undefined}
				>
					{skill?.qualified ? percent(skill.confidence) : "—"}
				</Typography.Mono>
			</Flex.Column>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Edge
				</Typography.Label>
				<Typography.Mono
					size="lg"
					tone={!skill?.qualified ? "f1" : skill.mean > 0 ? "accent" : "f2"}
					data-agent-edge={skill?.qualified ? String(skill.mean) : ""}
					title="Mean forward return over one disjoint measurement window, as a fraction of the policy lane's starting capital"
				>
					{skill?.qualified ? basis(skill.mean) : "—"}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};
