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
				Learned:{" "}
				<span data-metric="decisions" data-format="integer">
					0
				</span>{" "}
				· Evaluated:{" "}
				<span data-metric="evaluated" data-format="integer">
					0
				</span>{" "}
				· Accuracy:{" "}
				<span data-metric="accuracy" data-format="percent">
					—
				</span>
			</Typography.Mono>
		</Section.Body>
	</Section>
);
