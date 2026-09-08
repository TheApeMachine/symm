import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { action, amount, basis, percent } from "./format";
import type { LearningView, Prior } from "./state";

export const PriorFacts = ({ prior }: { prior: Prior }) => (
	<Typography.Mono size="s">
		{prior.Defined
			? `${basis(prior.Mean)} · variance ${prior.VarianceDefined ? prior.Variance.toExponential(3) : "unestimable"} · support ${amount(prior.Support)} · retained evidence ${prior.EvidenceAuthority === undefined ? "unavailable" : percent(prior.EvidenceAuthority)} · authority ${percent(prior.Authority)} · depth ${prior.Depth ?? "unavailable"}/${prior.ContextLength ?? "unavailable"} · samples ${prior.Samples} · pending ${prior.Pending ?? "unavailable"}`
			: "No completed evidence"}
	</Typography.Mono>
);

export const KnowledgePanel = ({ view }: { view: LearningView | null }) => (
 <Section fit="content">
  <Section.Header title="Learned context evidence" meta="Readings used for the selected symbol" />
  {view?.candidates?.map(candidate => <Section.Body className="p-3" key={`${candidate.kind}-${candidate.power}-${candidate.reduce}`}>
   <Typography.Mono>{action(candidate.kind, candidate.power, candidate.reduce)}</Typography.Mono>
   <PriorFacts prior={candidate.prior} />
  </Section.Body>)}
 </Section>
);
