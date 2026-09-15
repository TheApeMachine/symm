import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";

export const KnowledgePanel = () => (
	<Section fit="content">
		<Section.Header
			title="Learned context evidence"
			meta="Readings used for the selected symbol"
		/>
		<Section.Body className="p-3">
			<Typography.Mono>
				Confidence: <span data-metric="confidence" data-format="percent">0.0%</span> · Contrast: <span data-metric="contrast" data-format="bits">0.00 bits</span> · Support: <span data-metric="support" data-format="integer">0</span> observations
			</Typography.Mono>
		</Section.Body>
	</Section>
);
