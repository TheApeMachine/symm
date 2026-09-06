import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { action, amount, basis, percent } from "./format";
import type { LearningView, Prior } from "./state";

export const PriorFacts = ({ prior }: { prior: Prior }) => (
	<Typography.Mono size="s">
		{prior.Defined
			? `${basis(prior.Mean)}/s · variance ${prior.VarianceDefined ? prior.Variance.toExponential(3) : "unestimable"} · support ${amount(prior.Support)} · retained evidence ${prior.EvidenceAuthority === undefined ? "unavailable" : percent(prior.EvidenceAuthority)} · authority ${percent(prior.Authority)} · depth ${prior.Depth ?? "unavailable"}/${prior.ContextLength ?? "unavailable"} · samples ${prior.Samples} · pending ${prior.Pending ?? "unavailable"}`
			: "No completed evidence"}
	</Typography.Mono>
);

export const KnowledgePanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Shared knowledge and symbol specialization"
			meta="Alternative specificity levels"
		/>
		{view?.candidates?.map(
			(candidate) =>
				candidate.knowledge && (
					<Section.Body
						key={`${candidate.kind}-${candidate.power}-${candidate.reduce}`}
						className="space-y-2 border-(--line) border-b p-3"
					>
						<Typography.Label>
							{action(candidate.kind, candidate.power, candidate.reduce)} ·
							selected {candidate.knowledge.scope}
						</Typography.Label>
						<Typography.Label tone="f4">Global</Typography.Label>
						<PriorFacts prior={candidate.knowledge.global} />
						<Typography.Label tone="f4">This symbol</Typography.Label>
						<PriorFacts prior={candidate.knowledge.symbol} />
					</Section.Body>
				),
		)}
		<Typography.Mono className="p-3" size="s">
			Warmup: {view?.warmup?.targetUnavailable ?? 0} without a recoverable absolute return · {view?.warmup?.resolved ?? "unavailable"} complete experiences ·{" "}
			{view?.warmup?.unconditioned ?? "unavailable"} without historical quantity
			identities · {view?.warmup?.portfolioUnavailable ?? "unavailable"} without
			portfolio inputs. Historical knowledge grants no live entry authority.
		</Typography.Mono>
	</Section>
);
