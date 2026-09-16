import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";

export const SkillPanel = () => (
	<div className="flex flex-col">
		<Section fit="content">
			<Section.Header
				title="Cognition model skill"
				meta={<span data-l="skill-meta">forward testing · learning</span>}
			/>
			<Flex.Column className="gap-px border-(--line) border-b p-3">
				<Typography.Label size="s" tone="f4" weight="normal">
					Mean forward trade return
				</Typography.Label>
				<Typography.Mono size="lg" tone="accent" data-metric="edge" data-format="basis">
					0.0 bp
				</Typography.Mono>
				<Typography.Mono size="s" tone="f4">
					Predictions are scored against later bid/ask quotes with fees. These returns do not model depth or order fills.
				</Typography.Mono>
			</Flex.Column>
			<Flex.Column className="gap-px border-(--line) border-b p-3">
				<Typography.Label size="s" tone="f4" weight="normal">
					Prediction accuracy
				</Typography.Label>
				<Typography.Mono size="lg" tone="f1" data-metric="accuracy" data-format="percent">
					0.0%
				</Typography.Mono>
				<Typography.Mono size="s" tone="f4">
					Causal precursor direction accuracy compared to honest tape resolution.
				</Typography.Mono>
			</Flex.Column>
		</Section>

		<Section fit="content">
			<Section.Header
				title="Precursor memory"
				meta="Learned associations"
			/>
			<Flex.Column className="gap-px border-(--line) border-b p-3">
				<Typography.Label size="s" tone="f4" weight="normal">
					Learned situations
				</Typography.Label>
				<Typography.Mono size="lg" tone="accent" data-metric="decisions" data-format="integer">
					0
				</Typography.Mono>
				<Typography.Mono size="s" tone="f4">
					Associations stored across continuous market precursor memory.
				</Typography.Mono>
			</Flex.Column>
		</Section>
	</div>
);
