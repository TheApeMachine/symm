import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";

export const CandidateReview = () => (
	<Section fit="content">
		<Section.Header
			title="Completed tape evaluations"
			meta="Causal forward outcomes"
		/>
		<Section.Body className="space-y-2 p-3">
			<Typography.Mono>
				<span data-metric="resolved" data-format="integer">0</span> completed evaluations · Mean edge: <span data-metric="edge" data-format="basis">0.0 bp</span> · Win rate: <span data-metric="win_rate" data-format="percent">0.0%</span>
			</Typography.Mono>
			<Typography.Mono tone="f4">
				Continuous forward test evaluations against live order book outcomes.
			</Typography.Mono>
		</Section.Body>
	</Section>
);
