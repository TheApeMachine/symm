import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { basis, clock, percent } from "./format";
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

/*
SkillPanel shows the promotion machinery in full: the estimate, the bar it has
to clear, and the authority it currently justifies. The bar is deliberately
visible — an operator should never have to guess what "good enough to trade"
means, and the asymmetry between promotion and demotion is stated rather than
inferred from behaviour.
*/
export const SkillPanel = ({ view }: { view: LearningView | null }) => {
	const skill = view?.skill;

	if (!skill) {
		return (
			<Section fit="content">
				<Section.Header title="Agent skill" meta="waiting for the workspace" />
			</Section>
		);
	}

	const promotable = skill.qualified && skill.lowerBound > 0;

	return (
		<Section fit="content">
			<Section.Header
				title="Agent skill"
				meta={`${skill.samples.toLocaleString()} resolved policy decisions`}
			/>
			<Flex.Row className="items-center gap-3 border-(--line) border-b p-3">
				<Badge
					label={skill.mode === "trading" ? "trading" : "learning"}
					variant={
						skill.mode !== "trading"
							? "info"
							: skill.account === "real"
								? "error"
								: "success"
					}
					dot
					size="m"
				/>
				<Badge
					label={`account · ${skill.account}`}
					variant="disabled"
					size="m"
				/>
				<Typography.Mono size="s" tone="f3">
					{skill.reason} · since {clock(skill.since)}
				</Typography.Mono>
			</Flex.Row>
			<Flex.Column>
				<Reading
					label="Mean forward return per decision"
					value={skill.defined ? basis(skill.mean) : "no evidence"}
					note="Account change over one disjoint measurement window, as a fraction of starting capital"
					tone={skill.defined && skill.mean > 0 ? "accent" : "f1"}
				/>
				<Reading
					label={`Lower bound at ${skill.sigma}σ`}
					value={
						skill.qualified
							? basis(skill.lowerBound)
							: skill.varianceDefined
								? "evidence below the confidence floor"
								: "dispersion not estimable"
					}
					note="Promotion requires this above zero: the edge must exceed its own measurement error"
					tone={promotable ? "accent" : "f1"}
				/>
				<Reading
					label="Confidence the edge is positive"
					value={skill.qualified ? percent(skill.confidence) : "—"}
					note="Empirical normal probability over these observations — not a calibrated forecast, and not a win rate"
				/>
				<Reading
					label="Evidence"
					value={
						skill.defined
							? `${skill.support.toFixed(1)} effective of ${skill.samples.toLocaleString()}`
							: "none"
					}
					note={`Disjoint forward windows only — decisions issue far faster than a window closes, and overlapping ones are not independent evidence. Kish effective size under issue-time authority · ${skill.memory} window retention`}
				/>
				<Reading
					label="Outcome sign"
					value={`${skill.wins.toLocaleString()} positive · ${skill.losses.toLocaleString()} negative`}
					note="Counts of admitted windows by sign; the mean above is what promotion reads, not this tally"
				/>
				<Reading
					label="Transitions"
					value={`${skill.promotions} went live · ${skill.demotions} fell back`}
					note="Going live needs a positive lower bound; falling back needs only a non-positive mean. It falls back to learning, never to nothing"
				/>
				<Reading
					label={`Intents sent to the ${skill.account} account`}
					value={view.dispatched.toLocaleString()}
					note="Zero while calibrating. The policy lane's own wallet is a simulation running alongside, not this account"
				/>
				{view.hasExecution ? (
					<>
						<Reading
							label="Intents the account acted on"
							value={view.execution.submitted.toLocaleString()}
							note={`${view.execution.queued} waiting on the venue · orders are placed off the deciding path, so a slow venue cannot stall the pipeline`}
						/>
						<Reading
							label="Disagreed with the account"
							value={view.execution.diverged.toLocaleString()}
							note="The agent decides from its simulated wallet. An entry on a symbol the account already holds, or an exit on one it never opened, is left alone rather than forced"
						/>
						<Reading
							label="Dropped and refused"
							value={`${view.execution.dropped.toLocaleString()} dropped · ${view.execution.failed.toLocaleString()} refused`}
							note={
								view.execution.lastFailure ||
								"Dropped means the venue was slower than the agent was deciding, so an intent went stale before it could be placed. Refused means an order was actually rejected"
							}
							tone={view.execution.failed > 0 ? "f2" : "f1"}
						/>
					</>
				) : (
					<Reading
						label="Intents the account did not accept"
						value={view.rejected.toLocaleString()}
						note={
							view.rejection ||
							"No account is attached, so nothing is being placed"
						}
					/>
				)}
			</Flex.Column>
		</Section>
	);
};
