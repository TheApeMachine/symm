import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Sparkline } from "#/components/ui/sparkline";
import { Typography } from "#/components/ui/typography";
import {
	action,
	amount,
	basis,
	clock,
	duration,
	percent,
	rational,
} from "./format";
import { PriorFacts } from "./knowledge-panel";
import type { AccountLearning, LearningView } from "./state";

const AccountPanel = ({
	title,
	account,
}: {
	title: string;
	account: AccountLearning;
}) => (
	<Section fit="content">
		<Section.Header
			title={title}
			meta={`${account.resolved} completed allocation windows`}
		/>
		<Flex.Column className="gap-2 p-3">
			{!account.state.complete && (
				<Typography.Mono role="status">
					{account.state.reason || "Awaiting an authoritative mark"}
				</Typography.Mono>
			)}
			<Typography.Mono>
				Quote cash {rational(account.state.actualCash)} · reserved{" "}
				{rational(account.state.committed)} · available{" "}
				{rational(account.state.cash)}
			</Typography.Mono>
			<Typography.Mono>
				Marked equity{" "}
				{account.state.mark.version
					? amount(account.state.mark.equity)
					: "unavailable"}{" "}
				· producer v{account.state.mark.version} ·{" "}
				{clock(account.state.mark.at)}
			</Typography.Mono>
			<Typography.Mono>
				Wallet profit{" "}
				{account.state.mark.version
					? amount(account.outcome.totalReward)
					: "unavailable"}{" "}
				· rate{" "}
				{account.outcome.hasRate ? amount(account.outcome.rate) : "unmeasured"}
				/s · allocation target{" "}
				{account.resolved ? `${basis(account.target)}/s` : "unresolved"}
			</Typography.Mono>
			<Typography.Mono>
				MFE {amount(account.mfe)} · MAE {amount(account.mae)} · allocation
				duration {duration(account.holdingNs)}
			</Typography.Mono>
			<Typography.Mono>
				First positive {duration(account.timeToPositiveNs)} · recovered
				breakeven {duration(account.timeToBreakevenNs)}
			</Typography.Mono>
			<Typography.Mono size="s">
				Pending allocation {account.pending || "none"}
			</Typography.Mono>
			<Sparkline
				points={(account.trajectory || []).map((mark) => mark.equity)}
				role="img"
				aria-label={`${title} equity by successive observation`}
			/>
			<Flex.Row className="flex-wrap gap-3">
				{account.trajectory?.map((mark) => (
					<Typography.Mono key={mark.version} size="s">
						{clock(mark.at)}: {amount(mark.equity)}
					</Typography.Mono>
				))}
			</Flex.Row>
			{Object.entries(account.state.positions || {}).map(
				([symbol, quantity]) => (
					<Typography.Mono key={symbol}>
						{symbol}: {rational(quantity)} held
					</Typography.Mono>
				),
			)}
		</Flex.Column>
	</Section>
);

export const CapitalPanel = ({ view }: { view: LearningView | null }) => {
	const capital = view?.capital;
	if (!capital || !view)
		return (
			<Typography.Mono className="p-3">
				Awaiting shared-capital state.
			</Typography.Mono>
		);
	return (
		<Flex.Column>
			<Section.Header
				title="Learned finite-capital competition"
				meta={`${capital.decisions} account decisions`}
			/>
			<Flex.Column className="gap-2 p-3">
				<Flex.Row className="flex-wrap gap-3">
					<Badge
						label={`Skill allows increase: ${view.skill.mode === "trading" ? "yes" : "no"}`}
						variant="info"
					/>
					<Badge
						label={`Realization allows increase: ${view.realizationAllowed ? "yes" : "no"}`}
						variant="info"
					/>
					<Badge label="Genuine reductions allowed" variant="info" />
					<Badge
						label={`Effective mode: ${view.authorizedMode || "learning"}`}
						variant="info"
					/>
				</Flex.Row>
				<Typography.Mono>
					Last allocation: {capital.choice.symbol || "cash"} ·{" "}
					{action(capital.choice.kind, capital.choice.power, false)} · current
					demand {rational(capital.demand)}
				</Typography.Mono>
				<PriorFacts prior={capital.prior} />
				<Typography.Mono>
					Pre-submission refusals {view.execution.refused ?? 0} ·{" "}
					{view.execution.lastRefusal || "none reported"}
				</Typography.Mono>
				<Typography.Mono>
					Last learned expectation{" "}
					{capital.prior.Defined
						? `${basis(capital.prior.Mean)}/s`
						: "no evidence"}{" "}
					· last actual outcome{" "}
					{capital.actual.resolved
						? `${basis(capital.actual.target)}/s`
						: "unresolved"}{" "}
					· last exploration outcome{" "}
					{capital.exploration.resolved
						? `${basis(capital.exploration.target)}/s`
						: "unresolved"}
				</Typography.Mono>
			</Flex.Column>
			<AccountPanel title="Actual account teacher" account={capital.actual} />
			<AccountPanel
				title="One shared exploration wallet"
				account={capital.exploration}
			/>
			<Section fit="content">
				<Section.Header
					title="Prospective capital claims"
					meta={`${capital.candidates?.length || 0} present claims`}
				/>
				{capital.candidates?.map((candidate) => (
					<Flex.Column
						key={candidate.id}
						className="gap-2 border-(--line) border-b p-3"
					>
						<Typography.Label>
							{candidate.symbol} ·{" "}
							{action(candidate.action, candidate.power, false)} ·{" "}
							{candidate.state}
						</Typography.Label>
						<Typography.Mono>
							Quantity {rational(candidate.quantity)} · fee-inclusive demand{" "}
							{rational(candidate.notional)} · reference{" "}
							{rational(candidate.reference)} · fee{" "}
							{rational(candidate.feeRate)}
						</Typography.Mono>
						<Typography.Mono>
							{candidate.current ? "Current" : "Stale"} · age{" "}
							{duration(candidate.ageNs)} / horizon{" "}
							{duration(candidate.horizonNs)} · Grid v{candidate.gridVersion} ·
							issue authority {percent(candidate.authority)}
						</Typography.Mono>
						<Typography.Mono>
							Selected {candidate.scope} · context [
							{candidate.context?.join(", ")}]
						</Typography.Mono>
						<PriorFacts prior={candidate.prior} />
						<Typography.Label tone="f4">Global</Typography.Label>
						<PriorFacts prior={candidate.global} />
						<Typography.Label tone="f4">Symbol</Typography.Label>
						<PriorFacts prior={candidate.symbolPrior} />
						<Typography.Mono size="s">
							Candidate {candidate.id} · local decision {candidate.decision} ·
							issued {clock(candidate.at)}
						</Typography.Mono>
					</Flex.Column>
				))}
				{capital.outcomes?.map((outcome, index) => (
					<Typography.Mono
						key={`${outcome.id}-${outcome.at}-${index}`}
						className="border-(--line) border-b p-3"
						size="s"
					>
						{clock(outcome.at)} · {outcome.id} · {outcome.state} ·{" "}
						{outcome.detail}{" "}
						{outcome.portfolioId && `· allocation ${outcome.portfolioId}`}
					</Typography.Mono>
				))}
			</Section>
		</Flex.Column>
	);
};
