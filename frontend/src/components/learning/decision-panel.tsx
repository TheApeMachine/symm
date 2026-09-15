import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";

export const ForwardPanel = () => (
	<Section fit="content">
		<Section.Header
			title="Forward evaluation"
			meta={<span data-l="forward-meta">0 completed evaluations</span>}
		/>
		<Section.Body className="space-y-2 p-3">
			<Typography.Mono>
				Evaluated decisions: <span data-metric="resolved" data-format="integer">0</span> · Win rate: <span data-metric="win_rate" data-format="percent">0.0%</span> · Mean edge: <span data-metric="edge" data-format="basis">0.0 bp</span>
			</Typography.Mono>
			<Typography.Mono>
				Prediction accuracy: <span data-metric="accuracy" data-format="percent">0.0%</span>
			</Typography.Mono>
			<Typography.Mono tone="f4">
				Continuous causal evaluation streams verified predictions against honest market outcomes.
			</Typography.Mono>
		</Section.Body>
	</Section>
);

export const ImpulsePanel = () => (
	<Section fit="content">
		<Section.Header
			title="Current impulse & precursor metrics"
			meta={<span data-l="impulse-meta">causal precursor active</span>}
		/>
		<Section.Body className="p-3">
			<Typography.Mono>
				<span data-metric="ambiguity" data-format="spread">Spread 0.000</span> · <span data-metric="surprisal" data-format="surprisal">Surprisal 0.00 nat</span>
			</Typography.Mono>
		</Section.Body>
	</Section>
);

export const CandidatePanel = () => (
	<Section fit="content">
		<Section.Header
			title="Model action evaluation"
			meta="Causal policy candidates"
		/>
		<Section.Body className="p-3">
			<Typography.Mono>
				Policy decision: <span data-metric="action" data-format="action">WAIT</span> · Conf <span data-metric="confidence" data-format="percent">0.0%</span> · Contrast <span data-metric="contrast" data-format="bits">0.00 bits</span> · Edge <span data-metric="edge" data-format="basis">0.0 bp</span>
			</Typography.Mono>
		</Section.Body>
	</Section>
);

export const InfluencePanel = () => (
	<Section fit="content">
		<Section.Header
			title="Precursor discovery & associations"
			meta="Evidence from tape cognition"
		/>
		<Section.Body className="p-3">
			<Typography.Mono>
				Contrast <span data-metric="contrast" data-format="bits">0.00 bits</span> · <span data-metric="surprisal" data-format="surprisal">Surprisal 0.00 nat</span> · <span data-metric="ambiguity" data-format="spread">Spread 0.000</span>
			</Typography.Mono>
			<Typography.Mono tone="f4" className="mt-2">
				Direct precursor associations formed across parallel cognitive learners.
			</Typography.Mono>
		</Section.Body>
	</Section>
);
