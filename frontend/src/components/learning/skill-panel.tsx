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

	if (skill?.defined) {
		return (
			<Section fit="content">
				<Section.Header
					title="Agent skill"
					meta={`${skill.samples} completed decisions`}
				/>
				<Reading
					label="Mean completed decision benefit"
					value={basis(skill.mean)}
					note="Completed tape evaluations as a fraction of starting capital. Negative outcomes remain negative."
				/>
				<Reading
					label="Outcome signs"
					value={`${skill.wins} positive · ${skill.losses} negative`}
					note="These decisions can overlap in time; the count is not independent statistical evidence."
				/>
				<Reading
					label="Wallet performance"
					value={
						view?.lanes?.find((lane) => lane.mode === "policy")
							? amount(
									view.lanes.find((lane) => lane.mode === "policy")?.profit ?? 0,
								)
							: "unmeasured"
					}
					note="Net change in the consolidated-model agent's wallet, including fees."
				/>
			</Section>
		);
	}

	return (
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
				note="Associations stored across every learner. A situation is a sequence of regions the tape actually showed."
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
				note="The grid forms from co-activation on the tape. Recognition is what each learner answers when asked about a situation it has seen."
			/>
		</Section>
	);
};
