import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import {
	ImpulseBars,
	InfluenceGrid,
	MeasurementWindow,
	WalletBars,
} from "./charts";
import { action, amount, basis, duration, percent } from "./format";
import type { LearningView } from "./state";

export const ForwardPanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Forward evaluation"
			meta={`${view?.resolved ?? 0} completed decisions`}
		/>
		<Section.Body className="space-y-2 p-3">
			{view?.agents.map((member) => (
				<Typography.Mono key={member.id}>
					Agent {member.id + 1} · {String(member.reading?.samples ?? 0n)} graded
					· {String(member.wins)} positive · {String(member.losses)} negative ·{" "}
					{String(member.pending)} pending
				</Typography.Mono>
			))}
			<Typography.Mono>
				Each decision retains its issue-time context and economics until the
				durable tape can evaluate it. Wallet changes and elapsed time supply
				interim feedback.
			</Typography.Mono>
		</Section.Body>
	</Section>
);

export const ImpulsePanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Current impulse & precursor history"
			meta={
				view?.precursorDepth !== undefined && view.precursorDepth > 0
					? `precursor depth ${view.precursorDepth} · ${(view.precursorHistory?.length ?? 0) + 1} states`
					: view?.horizonNs
						? `scored over ${duration(view.horizonNs)}`
						: "causal precursor active"
			}
		/>
		<MeasurementWindow view={view} />
		<ImpulseBars impulse={view?.impulse ?? null} />
		<Section.Body className="overflow-x-auto">
			<table className="w-full text-left font-mono text-xs">
				<thead className="text-(--f4)">
					<tr>
						{[
							"Rank",
							"Token",
							"Context prefix",
							"Strength",
							"Authority",
							"Cells",
						].map((label) => (
							<th key={label} className="p-3 font-normal">
								{label}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{view?.impulse?.map((token, index) => (
						<tr key={token.token} className="border-(--line) border-t">
							<td className="p-3 text-(--acc)">{index + 1}</td>
							<td className="p-3">#{token.token}</td>
							<td className="p-3">
								<Typography.Mono tone="f1">{token.source}</Typography.Mono>
								<Typography.Mono tone="f3"> / {token.label}</Typography.Mono>
							</td>
							<td className="p-3">{amount(token.strength)}</td>
							<td className="p-3">{percent(token.authority)}</td>
							<td className="p-3">{token.members}</td>
						</tr>
					))}
					{!view?.impulse?.length && (
						<tr>
							<td className="p-3 text-(--f3)" colSpan={6}>
								No evidenced activity yet. The agent conditions on nothing and
								waits.
							</td>
						</tr>
					)}
				</tbody>
			</table>
		</Section.Body>
	</Section>
);

/*
CandidatePanel shows the feasible actions at the current impulse with the
evidence recalled for each, so a chosen action can be read against the ones it
beat. An undefined prior is not a zero: it means this action has never
completed here, which is exactly why exploration reaches for it.
*/
export const CandidatePanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Feasible actions at this impulse"
			meta={`${view?.candidates?.length ?? 0} candidates · policy lane context`}
		/>
		<Section.Body className="overflow-x-auto">
			<table className="w-full text-left font-mono text-xs">
				<thead>
					<tr>
						{[
							"Action",
							"Benefit",
							"Dispersion",
							"Support",
							"Authority",
							"Samples",
							"Evidence",
						].map((label) => (
							<th className="p-3" key={label}>
								{label}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{view?.candidates?.map((candidate) => (
						<tr
							key={`${candidate.kind}-${candidate.power}-${candidate.reduce}`}
						>
							<td className="p-3">
								{action(candidate.kind, candidate.power, candidate.reduce)}{" "}
								{candidate.selected ? "· chosen" : ""}
							</td>
							<td className="p-3">
								{candidate.prior.Defined
									? basis(candidate.prior.Mean)
									: "unmeasured"}
							</td>
							<td className="p-3">
								{candidate.prior.VarianceDefined
									? basis(Math.sqrt(candidate.prior.Variance))
									: "unmeasured"}
							</td>
							<td className="p-3">{amount(candidate.prior.Support)}</td>
							<td className="p-3">{percent(candidate.prior.Authority)}</td>
							<td className="p-3">{candidate.prior.Samples}</td>
							<td className="p-3">
								{candidate.prior.Provisional
									? "interim wallet feedback"
									: candidate.prior.Defined
										? "completed tape"
										: "unmeasured"}
							</td>
						</tr>
					))}
				</tbody>
			</table>
		</Section.Body>
	</Section>
);

export const InfluencePanel = ({ view }: { view: LearningView | null }) => {
	const influence = (view?.influence ?? [])
		.filter((entry) => entry.prior.Defined)
		.slice(0, 40);

	return (
		<Section fit="content">
			<Section.Header
				title="What is driving which action"
				meta={`${view?.influence?.length ?? 0} measured associations`}
			/>
			<InfluenceGrid influence={view?.influence ?? null} />
			<Section.Body className="overflow-x-auto">
				<table className="w-full text-left font-mono text-xs">
					<thead className="text-(--f4)">
						<tr>
							{[
								"Context prefix",
								"Action",
								"Mean outcome",
								"Support",
								"Authority",
								"Samples",
							].map((label) => (
								<th key={label} className="p-3 font-normal">
									{label}
								</th>
							))}
						</tr>
					</thead>
					<tbody>
						{influence.map((entry) => (
							<tr
								key={`${entry.token}-${entry.action}`}
								className="border-(--line) border-t"
							>
								<td className="p-3">
									<Typography.Mono tone="f1">
										{entry.source || `#${entry.token}`}
									</Typography.Mono>
									<Typography.Mono tone="f3"> / {entry.label}</Typography.Mono>
								</td>
								<td className="p-3 text-(--acc)">{entry.action}</td>
								<td className="p-3">{basis(entry.prior.Mean)}</td>
								<td className="p-3">{amount(entry.prior.Support)}</td>
								<td className="p-3">{percent(entry.prior.Authority)}</td>
								<td className="p-3">{entry.prior.Samples}</td>
							</tr>
						))}
						{influence.length === 0 && (
							<tr>
								<td className="p-3 text-(--f3)" colSpan={6}>
									No decision has resolved yet. Evidence appears once a
									measurement window closes.
								</td>
							</tr>
						)}
					</tbody>
				</table>
			</Section.Body>
			<Typography.Mono className="px-3 pb-3 text-(--f4)">
				Association under the agent's own exploration, not a controlled
				comparison: a quantity that is hot whenever the tape moves appears
				alongside good and bad outcomes alike, and overlapping return windows
				make these observations correlated.
			</Typography.Mono>
		</Section>
	);
};

/*
DeskPanel is the parallel trader population the shared memory is consolidated
from. Each trader owns a wallet and makes its own choices on the same
development; the tape later settles which of them read it right. This is the
"multiple agents, one model" surface: the wallets are separate, the memory they
write into is not.
*/
export const DeskPanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Parallel traders"
			meta={
				view?.desk
					? `${view.desk.traders.length} wallets · ${view.desk.settled} verdicts settled · completed tape evaluations`
					: "Awaiting the desk"
			}
		/>
		<Section.Body className="overflow-x-auto">
			<table className="w-full text-left font-mono text-xs">
				<thead className="text-(--f4)">
					<tr>
						{[
							"Trader",
							"Wealth",
							"Quality",
							"Decisions",
							"Fills",
							"Graded",
							"Pending grades",
							"Holding",
						].map((label) => (
							<th key={label} className="p-3 font-normal">
								{label}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{view?.desk?.traders.map((trader) => (
						<tr key={trader.id} className="border-(--line) border-t">
							<td className="p-3">
								<Typography.Mono tone="accent">
									trader {trader.id + 1}
								</Typography.Mono>
							</td>
							<td
								className={`p-3 ${trader.wealth < 0 ? "text-error" : "text-success"}`}
							>
								{trader.observed > 0 || trader.fills > 0
									? basis(trader.wealth)
									: "unvalued"}
							</td>
							<td className="p-3">
								{trader.observed > 0 ? basis(trader.quality) : "—"}
							</td>
							<td className="p-3">{trader.decisions}</td>
							<td className="p-3">{trader.fills}</td>
							<td className="p-3">{trader.graded}</td>
							<td className="p-3">{trader.open}</td>
							<td className="p-3">{trader.holding}</td>
						</tr>
					))}
					{!view?.desk?.traders.length && (
						<tr>
							<td className="p-3 text-(--f3)" colSpan={8}>
								No traders have been woken by the market yet.
							</td>
						</tr>
					)}
				</tbody>
			</table>
		</Section.Body>
		<Typography.Mono className="px-3 pb-3 text-(--f4)">
			Wealth is the trader's own wallet marked against the current executable
			book. Quality only moves when the tape settles a decision, so the two can
			disagree while a position is still open. Trader 1 follows the shared
			memory; the others spread across the rest of what is feasible so every
			move is tried by somebody.
		</Typography.Mono>
	</Section>
);

/* LanePanel keeps every cloned account's economics separate and legible. */
export const LanePanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Independent wallets"
			meta={
				view
					? `${view.initialCapital} starting cash in each lane`
					: "Awaiting account economics"
			}
		/>
		<WalletBars lanes={view?.lanes ?? null} />
		<Section.Body className="overflow-x-auto">
			<table className="w-full text-left font-mono text-xs">
				<thead className="text-(--f4)">
					<tr>
						{[
							"Lane",
							"Action",
							"Cash",
							"Selected-symbol quantity",
							"Fees",
							"P&L",
							"Fills",
							"Learned / pending",
						].map((label) => (
							<th key={label} className="p-3 font-normal">
								{label}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{view?.lanes?.map((lane) => (
						<tr key={lane.lane} className="border-(--line) border-t">
							<td className="p-3">
								<Flex.Row align="center" gap={4}>
									<Typography.Mono tone="accent">
										{lane.mode} {lane.lane + 1}
									</Typography.Mono>
								</Flex.Row>
							</td>
							<td className="p-3">
								{action(
									lane.action.kind,
									lane.action.power,
									lane.action.reduce,
								)}
							</td>
							<td className="p-3">{amount(Number(lane.cash))}</td>
							<td className="p-3">{amount(Number(lane.quantity))}</td>
							<td className="p-3">{amount(Number(lane.fees))}</td>
							<td
								className={`p-3 ${lane.profit < 0 ? "text-error" : "text-success"}`}
							>
								{amount(lane.profit)}
							</td>
							<td className="p-3">{lane.fills}</td>
							<td className="p-3">
								{lane.resolved} / {lane.unresolved}
							</td>
						</tr>
					))}
				</tbody>
			</table>
		</Section.Body>
		<Typography.Mono className="px-3 pb-3 text-(--f4)">
			P&L includes entry fees and liquidation at displayed bids, including exit
			fees. Each agent retains its own capital and positions. Positive completed
			exploration experience also trains the consolidated agent.
		</Typography.Mono>
	</Section>
);
