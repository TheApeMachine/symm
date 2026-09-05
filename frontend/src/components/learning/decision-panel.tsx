import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { action, amount, basis, duration, percent } from "./format";
import type { LearningView } from "./state";

/*
ImpulsePanel names the quantities that are hot right now, in the order the
next decision is conditioned on. A region's identity is the strongest
contributing quantity at its peak cell: it names where the basin peaked, and
the other members of that basin are not listed individually.
*/
export const ImpulsePanel = ({ view }: { view: LearningView | null }) => (
	<Section fit="content">
		<Section.Header
			title="Current impulse"
			meta={
				view?.horizonNs
					? `scored over ${duration(view.horizonNs)} · ${view.horizonEpochs} epochs of ${(view.epochMean ?? 0).toFixed(2)}s`
					: "horizon not measured yet"
			}
		/>
		<Section.Body className="overflow-x-auto">
			<table className="w-full text-left font-mono text-xs">
				<thead className="text-(--f4)">
					<tr>
						{[
							"Rank",
							"Token",
							"Quantity",
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
				<thead className="text-(--f4)">
					<tr>
						{[
							"Action",
							"Mean",
							"Dispersion",
							"Support",
							"Authority",
							"Samples",
							"State",
						].map((label) => (
							<th key={label} className="p-3 font-normal">
								{label}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{view?.candidates?.map((candidate) => (
						<tr
							key={`${candidate.kind}-${candidate.power}-${candidate.reduce}`}
							className={`border-(--line) border-t ${candidate.selected ? "bg-[color:color-mix(in_srgb,var(--acc)_8%,transparent)]" : ""}`}
						>
							<td className="p-3 text-(--acc)">
								{action(candidate.kind, candidate.power, candidate.reduce)}
							</td>
							<td className="p-3">
								{candidate.prior.Defined
									? basis(candidate.prior.Mean)
									: "no evidence"}
							</td>
							<td className="p-3">
								{candidate.prior.VarianceDefined
									? basis(Math.sqrt(candidate.prior.Variance))
									: "unestimable"}
							</td>
							<td className="p-3">{amount(candidate.prior.Support)}</td>
							<td className="p-3">{percent(candidate.prior.Authority)}</td>
							<td className="p-3">{candidate.prior.Samples}</td>
							<td className="p-3">
								{candidate.selected ? (
									<Badge label="policy choice" variant="success" size="xs" />
								) : candidate.prior.VarianceDefined ? (
									<Badge label="estimated" variant="info" size="xs" />
								) : (
									<Badge
										label="exploration target"
										variant="warning"
										size="xs"
									/>
								)}
							</td>
						</tr>
					))}
					{!view?.candidates?.length && (
						<tr>
							<td className="p-3 text-(--f3)" colSpan={7}>
								No executable action set yet — the book has not offered a
								feasible quantity.
							</td>
						</tr>
					)}
				</tbody>
			</table>
		</Section.Body>
		<Typography.Mono className="px-3 pb-3 text-(--f4)">
			Power is a bisection depth of the currently executable range, down to
			venue lot and cost minimums — not a chosen allocation percentage.
		</Typography.Mono>
	</Section>
);

/*
InfluencePanel is the discovery answer: which measured quantities have
accumulated outcome evidence for which actions. Ranking is by the prior's own
authority, so a large mean built on one observation cannot outrank a smaller
one that has been measured repeatedly.
*/
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
			<Section.Body className="overflow-x-auto">
				<table className="w-full text-left font-mono text-xs">
					<thead className="text-(--f4)">
						<tr>
							{[
								"Quantity",
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
		<Section.Body className="overflow-x-auto">
			<table className="w-full text-left font-mono text-xs">
				<thead className="text-(--f4)">
					<tr>
						{[
							"Lane",
							"Action",
							"Cash",
							"Inventory",
							"Episode fees",
							"Episode P&L",
							"Episodes",
							"Realized",
							"Lifetime fees",
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
									{lane.exhausted && (
										<Badge label="spent" variant="warning" size="xxs" />
									)}
								</Flex.Row>
							</td>
							<td className="p-3">
								{action(
									lane.action.kind,
									lane.action.power,
									lane.action.reduce,
								)}
								{lane.pending ? " · pending" : ""}
							</td>
							<td className="p-3">{amount(Number(lane.cash))}</td>
							<td className="p-3">{amount(Number(lane.quantity))}</td>
							<td className="p-3">{amount(Number(lane.fees))}</td>
							<td
								className={`p-3 ${lane.profit < 0 ? "text-error" : "text-success"}`}
							>
								{lane.complete ? amount(lane.profit) : "unvalued"}
							</td>
							<td className="p-3">{lane.episodes}</td>
							<td
								className={`p-3 ${lane.realized < 0 ? "text-error" : "text-success"}`}
							>
								{amount(lane.realized)}
							</td>
							<td className="p-3">{amount(lane.spent + Number(lane.fees))}</td>
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
			fees. Each lane owns its capital and a spent lane restarts on a fresh
			clone of the same known balance — episodes are separate accounts in
			sequence, never a balance anyone holds. The policy lane trades on
			completed exploration evidence.
		</Typography.Mono>
	</Section>
);
