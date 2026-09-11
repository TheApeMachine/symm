import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import {
	DecisionRing,
	DrivingActions,
	ImpulseBars,
	InfluenceGrid,
	MeasurementWindow,
	TraderQuality,
	WalletBars,
} from "./charts";
import { action, amount, basis, duration, percent } from "./format";
import type { LearningView } from "./state";

export const ForwardPanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Forward evaluation"
			meta={`${view?.resolved ?? 0} completed evaluations`}
		/>
		<DecisionRing view={view} />
		<TraderQuality view={view} />
		<Section.Body className="space-y-2 p-3">
			{view?.agents?.map((member) => (
				<Typography.Mono key={member.id}>
					{member.id === 0
						? `Main Agent (Policy Trader) · ${String(member.reading?.samples ?? 0n)} simulated trades graded · ${String(member.wins)} profitable · ${String(member.losses)} unprofitable · ${String(member.pending)} pending fills`
						: `Rehearsal Worker ${member.id + 1} · ${String(member.reading?.samples ?? 0n)} fragment steps graded · ${String(member.wins)} profitable · ${String(member.losses)} unprofitable · ${String(member.pending)} pending`}
				</Typography.Mono>
			))}
			<Typography.Mono tone="f4">
				The Main Agent executes forward-test trades evaluated against live market economics and fees.
				Parallel rehearsal workers contribute experience to the shared model across varying precursor horizons.
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
			title="Main agent action evaluation at current impulse"
			meta={`${view?.candidates?.length ?? 0} candidates · evaluated against action priors`}
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
			<DrivingActions influence={view?.influence ?? null} />
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
DeskPanel presents the Main Agent alongside the parallel rehearsal workers.
The Main Agent owns the execution wallet, carrying simulated economics and forward-testing
returns. Parallel rehearsal workers dedicate their compute to exploring actions
across fragmented observation histories.
*/
export const DeskPanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Main Agent & Rehearsal Workers"
			meta={
				view?.desk
					? `1 execution wallet (Main Agent) · ${Math.max(0, view.desk.traders.length - 1)} rehearsal workers · ${view.desk.settled} verdicts settled`
					: "Awaiting the desk"
			}
		/>
		<Section.Body className="overflow-x-auto">
			<table className="w-full text-left font-mono text-xs">
				<thead className="text-(--f4)">
					<tr>
						{[
							"Agent / Worker",
							"Role",
							"Wealth (Return)",
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
					{view?.desk?.traders.map((trader) => {
						const isMain = trader.id === 0;
						return (
							<tr key={trader.id} className="border-(--line) border-t">
								<td className="p-3">
									<Typography.Mono tone="accent">
										{isMain
											? "Agent 1 (Main Agent)"
											: `Agent ${trader.id + 1} (Rehearsal Worker)`}
									</Typography.Mono>
								</td>
								<td className="p-3">
									<Typography.Mono tone={isMain ? "f1" : "f3"}>
										{isMain
											? "Forward Test Trader (Policy)"
											: "Rehearsal Worker (Fragmented)"}
									</Typography.Mono>
								</td>
								<td
									className={`p-3 ${
										!isMain
											? "text-(--f3)"
											: trader.wealth < 0
												? "text-error"
												: "text-success"
									}`}
								>
									{isMain
										? trader.observed > 0 || trader.fills > 0
											? basis(trader.wealth)
											: "unvalued"
										: "— (rehearsal)"}
								</td>
								<td className="p-3">
									{trader.observed > 0 ? basis(trader.quality) : "—"}
								</td>
								<td className="p-3">{trader.decisions}</td>
								<td className="p-3">{isMain ? trader.fills : "0"}</td>
								<td className="p-3">{trader.graded}</td>
								<td className="p-3">{trader.open}</td>
								<td className="p-3">{isMain ? trader.holding : "none"}</td>
							</tr>
						);
					})}
					{!view?.desk?.traders.length && (
						<tr>
							<td className="p-3 text-(--f3)" colSpan={9}>
								No traders or workers have been woken by the market yet.
							</td>
						</tr>
					)}
				</tbody>
			</table>
		</Section.Body>
		<Typography.Mono className="px-3 pb-3 text-(--f4)">
			The Main Agent (Agent 1) carries execution capital, testing simulated trades against the live book. Parallel rehearsal workers (Agents 2+) dedicate their compute to exploring actions across fragmented observation histories and updating the shared associative memory. When the Main Agent demonstrates robust net-positive edge, it is promoted to live paper or real execution.
		</Typography.Mono>
	</Section>
);

/* LanePanel keeps the Main Agent's execution wallet separate from parallel rehearsal workers. */
export const LanePanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Execution Lanes & Channels"
			meta={
				view
					? `Lane 1: Main Agent Wallet ($${view.initialCapital || "10,000"}) · Lanes 2+: Rehearsal Workers`
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
							"Role",
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
					{view?.lanes?.map((lane) => {
						const isMain = lane.lane === 0;
						return (
							<tr key={lane.lane} className="border-(--line) border-t">
								<td className="p-3">
									<Flex.Row align="center" gap={4}>
										<Typography.Mono tone="accent">
											{isMain
												? "Lane 1 (Main Agent Policy)"
												: `Lane ${lane.lane + 1} (Rehearsal Worker)`}
										</Typography.Mono>
									</Flex.Row>
								</td>
								<td className="p-3">
									<Typography.Mono tone={isMain ? "f1" : "f3"}>
										{isMain ? "Forward Test Wallet" : "Rehearsal Worker"}
									</Typography.Mono>
								</td>
								<td className="p-3">
									{isMain
										? action(
												lane.action.kind,
												lane.action.power,
												lane.action.reduce,
											)
										: "fragment evaluation"}
								</td>
								<td className="p-3">
									{isMain ? amount(Number(lane.cash)) : "—"}
								</td>
								<td className="p-3">
									{isMain ? amount(Number(lane.quantity)) : "—"}
								</td>
								<td className="p-3">
									{isMain ? amount(Number(lane.fees)) : "—"}
								</td>
								<td
									className={`p-3 ${
										!isMain
											? "text-(--f3)"
											: lane.profit < 0
												? "text-error"
												: "text-success"
									}`}
								>
									{isMain ? amount(lane.profit) : "—"}
								</td>
								<td className="p-3">{isMain ? lane.fills : "0"}</td>
								<td className="p-3">
									{lane.resolved} / {lane.unresolved}
								</td>
							</tr>
						);
					})}
				</tbody>
			</table>
		</Section.Body>
		<Typography.Mono className="px-3 pb-3 text-(--f4)">
			The Main Agent exclusively manages Lane 1 with simulated execution capital, fees, and positions. Lanes 2+ represent parallel rehearsal workers exploring actions across fragmented observation histories.
		</Typography.Mono>
	</Section>
);
