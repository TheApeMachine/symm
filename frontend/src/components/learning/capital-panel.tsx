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
	const isTrading = member?.status === "trading";

	return (
		<Section fit="content">
			<Section.Header
				title="Main Agent Capital & Economics"
				meta={
					isTrading
						? "Paper/Real Execution Wallet · Main Agent"
						: "Simulated Forward Testing Wallet · Main Agent (Policy)"
				}
			/>
			<Section.Body className="space-y-3 p-3">
				<Typography.Mono>
					Cash {member ? String(member.cash) : "unmeasured"} · Equity{" "}
					{member ? String(member.equity) : "unmeasured"} · P&L{" "}
					{member ? String(member.profit) : "unmeasured"}
				</Typography.Mono>
				<Typography.Mono>
					Fees {member ? String(member.fees) : "unmeasured"} · Return{" "}
					{member ? basis(member.wealth) : "unmeasured"}
				</Typography.Mono>
				<Typography.Mono tone="f3">
					The Main Agent is the sole owner of capital and execution economics.
					Parallel precursor learners are decoupled from the wallet, learning
					directional precursors to upward, downward, and stagnant market movement.
					Simulated forward testing validates the Main Agent until it proves
					robustly net-positive before promotion to live paper/real trading.
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
					<Typography.Mono>No open positions.</Typography.Mono>
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
