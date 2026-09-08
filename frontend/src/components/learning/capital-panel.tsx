import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { basis } from "./format";
import type { LearningView } from "./state";

export const CapitalPanel = ({ view }: { view: LearningView | null }) => {
	/*
		The optional chain has to reach the index, not just the view. A state
		that arrived without agents is a state this panel has nothing to say
		about, and indexing it took the whole surface down rather than
		reporting the absence.
	*/
	const member = view?.agents?.[0];
	const positions = member?.positions ?? [];
	return (
		<Section fit="content">
			<Section.Header
				title="Consolidated model account"
				meta="Independent simulated wallet · agent 1"
			/>
			<Section.Body className="p-3 space-y-3">
				<Typography.Mono>
					Cash {member ? String(member.cash) : "unmeasured"} · Equity{" "}
					{member ? String(member.equity) : "unmeasured"} · P&L{" "}
					{member ? String(member.profit) : "unmeasured"}
				</Typography.Mono>
				<Typography.Mono>
					Fees {member ? String(member.fees) : "unmeasured"} · Return{" "}
					{member ? basis(member.wealth) : "unmeasured"}
				</Typography.Mono>
				<Typography.Mono>
					Positive explorer experience is consolidated into this model. Its own
					completed decisions measure its performance.
				</Typography.Mono>
				{positions.map((position) => (
					<Typography.Mono key={String(position.holding?.symbol)}>
						{String(position.holding?.symbol)} · {position.status} · quantity{" "}
						{String(position.holding?.qty)} · mark{" "}
						{String(position.holding?.mark ?? "unmeasured")} · P&L{" "}
						{String(position.holding?.pnl ?? "unmeasured")}
					</Typography.Mono>
				))}
				{member && positions.length === 0 && (
					<Typography.Mono>No positions.</Typography.Mono>
				)}
				{!member && (
					<Typography.Mono>
						No agent has reported yet, so there is no account to show.
					</Typography.Mono>
				)}
			</Section.Body>
		</Section>
	);
};
