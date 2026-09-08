import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { action, basis } from "./format";
import type { LearningView } from "./state";

export const CandidateReview = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Completed tape evaluations"
			meta="Latest result per agent"
		/>
		<Section.Body className="space-y-2 p-3">
			{view?.agents.map((member) => (
				<Typography.Mono key={member.id}>
					Agent {member.id + 1} ·{" "}
					{member.outcome
						? `${member.outcome.symbol} #${member.outcome.id} ${action(String(member.outcome.action?.kind), member.outcome.action?.power ?? 0, member.outcome.action?.reduce ?? false)} · ${basis(member.outcome.tape)}`
						: "awaiting a completed trade leg"}{" "}
					· {String(member.pending)} pending
				</Typography.Mono>
			))}
			<Typography.Mono>
				Grades use only trade legs read from S3 after a price reversal confirms
				their end. Tape opportunity return is separate from executable wallet
				profit.
			</Typography.Mono>
		</Section.Body>
	</Section>
);
