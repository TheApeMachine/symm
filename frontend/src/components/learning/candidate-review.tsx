import { useEffect, useState } from "react";
import { Alert } from "#/components/ui/alert";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { basis, clock } from "./format";
import { baseUrl, type LearningEvent, type LearningView } from "./state";

export const CandidateReview = ({ view }: { view: LearningView | null }) => {
	const [selected, setSelected] = useState("");
	const [events, setEvents] = useState<LearningEvent[]>([]);
	const [error, setError] = useState("");
	useEffect(() => {
		if (!selected) return;
		const controller = new AbortController();
		const read = async () => {
			try {
				const response = await fetch(
					`${baseUrl()}/learning/events?candidate=${encodeURIComponent(selected)}`,
					{ signal: controller.signal },
				);
				if (!response.ok)
					throw new Error(`Candidate journal: ${response.status}`);
				setEvents(await response.json());
				setError("");
			} catch (err) {
				if (!controller.signal.aborted) setError(String(err));
			}
		};
		void read();
		return () => controller.abort();
	}, [selected, view?.at]);
	const identities = Array.from(
		new Set([
			...(view?.capital?.candidates || []).map((candidate) => candidate.id),
			...(view?.capital?.outcomes || []).map((outcome) => outcome.id),
		]),
	);
	return (
		<Section fit="content">
			<Section.Header
				title="Prospective candidate review"
				meta="Later labels preserve the original input"
			/>
			<Typography.Mono className="p-3" size="s">
				Policy opportunity review uses spot observations. Futures evidence
				remains in Hindsight. Local outcomes describe the continuing executable
				policy path; they are not an isolated buy-and-hold counterfactual.
			</Typography.Mono>
			<Flex.Row className="flex-wrap gap-2 p-3">
				{identities.map((identity) => (
					<Button
						key={identity}
						variant={selected === identity ? "solid" : "quiet"}
						onClick={() => setSelected(identity)}
					>
						{identity}
					</Button>
				))}
			</Flex.Row>
			{error && <Alert>{error}</Alert>}
			{events.map((event) => (
				<Flex.Column
					key={`${event.id}-${event.kind}-${event.at}`}
					className="gap-2 border-(--line) border-t p-3"
				>
					<Typography.Mono>
						{clock(event.at)} · {event.kind} ·{" "}
						{event.allocation?.state ||
							event.candidateResult?.state ||
							event.mode}
					</Typography.Mono>
					{event.allocation?.detail && (
						<Typography.Mono>{event.allocation.detail}</Typography.Mono>
					)}
					{event.candidateResult?.detail && (
						<Typography.Mono>{event.candidateResult.detail}</Typography.Mono>
					)}
					{(event.kind === "resolved" ||
						event.kind === "portfolio_resolved") && (
						<Typography.Mono>
							{event.kind === "resolved" ? "Local policy" : "Shared account"}{" "}
							outcome {basis(event.target ?? 0)}/s · portfolio{" "}
							{event.portfolioId || "independent local experiment"}
						</Typography.Mono>
					)}
					<Typography.Mono size="s" className="break-all">
						{JSON.stringify(event)}
					</Typography.Mono>
				</Flex.Column>
			))}
		</Section>
	);
};
