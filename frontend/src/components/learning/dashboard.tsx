import { useState } from "react";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import {
	CandidatePanel,
	ForwardPanel,
	ImpulsePanel,
	InfluencePanel,
	LanePanel,
} from "./decision-panel";
import { action, amount, basis, clock, duration, percent } from "./format";
import { ImpulseMap } from "./map";
import { SkillPanel } from "./skill-panel";
import { type LearningEvent, useLearning } from "./state";

const KIND_TONE: Record<
	string,
	"info" | "success" | "warning" | "error" | "disabled"
> = {
	issued: "info",
	filled: "success",
	waited: "disabled",
	rejected: "warning",
	resolved: "success",
	recycled: "error",
};

/*
JournalEntry writes one persisted decision boundary. Each kind carries the
facts that kind actually has: a resolved record has a return target, a filled
record has an executed quantity and its fee, and a recycled record marks an
account that ran out of capital to act with.
*/
const JournalEntry = ({ event }: { event: LearningEvent }) => (
	<Flex.Column className="gap-1 border-(--line) border-b p-3">
		<Flex.Row align="center" gap={4} className="justify-between">
			<Typography.Mono size="s">
				{clock(event.at)} · {event.mode} {event.lane + 1}
			</Typography.Mono>
			<Badge
				label={event.kind}
				variant={KIND_TONE[event.kind] ?? "info"}
				size="xxs"
			/>
		</Flex.Row>
		<Typography.Mono tone="f1">
			#{event.id} {action(event.action, event.power, event.reduce)}
		</Typography.Mono>
		{event.kind === "issued" && (
			<Typography.Mono size="s" tone="f3">
				Requested {event.quantity} · authority {percent(event.authority)} ·
				horizon {duration(event.horizonNs)}
			</Typography.Mono>
		)}
		{event.kind === "filled" && (
			<Typography.Mono size="s" tone="f3">
				Filled {event.quantity} · fee {event.fee}
			</Typography.Mono>
		)}
		{event.kind === "rejected" && (
			<Typography.Mono size="s" tone="f3">
				No executable depth for {event.quantity}
			</Typography.Mono>
		)}
		{event.kind === "resolved" && (
			<Typography.Mono
				size="s"
				tone={(event.target ?? 0) >= 0 ? "accent" : "f2"}
			>
				Return {basis(event.target ?? 0)}{" "}
				{event.truncated
					? "over a window cut short by a spent account"
					: `over ${duration(event.horizonNs)}`}{" "}
				· prior now{" "}
				{event.prior?.Defined ? basis(event.prior.Mean) : "undefined"} on{" "}
				{event.prior?.Samples ?? 0}
			</Typography.Mono>
		)}
		{event.kind === "recycled" && (
			<Typography.Mono size="s" tone="f3">
				Account spent · episode {event.episode} begins on a fresh clone
			</Typography.Mono>
		)}
		{event.authorized && event.authorized !== "learning" && (
			<Typography.Mono size="s" tone="f4">
				Authority {event.authorized}
			</Typography.Mono>
		)}
	</Flex.Column>
);

type Tab = "decision" | "influence" | "forward" | "wallets";

const TABS: Array<{ key: Tab; label: string }> = [
	{ key: "decision", label: "Decision" },
	{ key: "influence", label: "Discovery" },
	{ key: "forward", label: "Forward test" },
	{ key: "wallets", label: "Wallets" },
];

export const LearningDashboard = () => {
	const [symbol, setSymbol] = useState("");
	const [tab, setTab] = useState<Tab>("decision");
	const { view, events, error } = useLearning(symbol);

	return (
		<Flex.Column className="h-full min-h-0 w-full">
			<Section.Header
				title="Forward learning"
				meta={
					view
						? `${view.steps.toLocaleString()} observations · ${view.decisions.toLocaleString()} decisions · ${view.resolved.toLocaleString()} outcomes · ${view.columns} numeric quantities`
						: "Connecting to the workspace"
				}
			/>
			{error && (
				<Typography.Mono role="alert" className="p-3 text-error">
					{error} · Last successful state remains visible.
				</Typography.Mono>
			)}
			<Flex className="min-h-0 flex-1 max-lg:flex-col">
				<Section className="w-52 shrink-0 border-(--line) border-r max-lg:h-36 max-lg:w-full">
					<Section.Header
						title="Universe"
						meta={`${view?.universe?.length ?? 0} keys`}
					/>
					<Section.Body className="p-2">
						{view?.universe?.map((entry) => (
							<Button
								key={entry.symbol}
								shape="block"
								variant={view.symbol === entry.symbol ? "solid" : "quiet"}
								tone="accent"
								onClick={() => setSymbol(entry.symbol)}
								aria-pressed={view.symbol === entry.symbol}
								className="mb-1 justify-between"
							>
								{entry.symbol}
								<Typography.Mono>{entry.decisions}</Typography.Mono>
							</Button>
						))}
					</Section.Body>
				</Section>

				<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-auto">
					<Section.Header
						title={view?.symbol || "Waiting for market observations"}
						meta={view?.status}
					/>
					<Flex.Row align="center" gap={6} className="flex-wrap px-3 pb-2">
						<Badge
							label={
								view?.skill?.mode === "trading"
									? `trading · ${view.skill.account}`
									: "learning"
							}
							variant={
								view?.skill?.mode !== "trading"
									? "info"
									: view.skill.account === "real"
										? "error"
										: "success"
							}
							dot
						/>
						<Typography.Mono size="s" tone="f3">
							Horizon {duration(view?.horizonNs ?? 0)} ·{" "}
							{view?.epochs?.toLocaleString() ?? 0} impulse epochs observed ·
							grid v{view?.gridVersion ?? 0}
						</Typography.Mono>
					</Flex.Row>
					<ImpulseMap
						points={view?.points ?? []}
						regions={view?.regions ?? []}
					/>

					<Flex.Row gap={2} className="border-(--line) border-b px-3 py-2">
						{TABS.map((entry) => (
							<Button
								key={entry.key}
								size="m"
								variant={tab === entry.key ? "solid" : "quiet"}
								tone="accent"
								onClick={() => setTab(entry.key)}
								aria-pressed={tab === entry.key}
							>
								{entry.label}
							</Button>
						))}
					</Flex.Row>

					{tab === "decision" && (
						<>
							<ImpulsePanel view={view} />
							<CandidatePanel view={view} />
						</>
					)}
					{tab === "influence" && <InfluencePanel view={view} />}
					{tab === "forward" && <ForwardPanel view={view} />}
					{tab === "wallets" && <LanePanel view={view} />}
				</Flex.Column>

				<Flex.Column className="w-96 shrink-0 overflow-auto border-(--line) border-l max-lg:w-full">
					<SkillPanel view={view} />
					<Section fit="content">
						<Section.Header title="Hot regions" meta="strongest first" />
						<Flex.Column className="gap-2 p-3">
							{view?.regions?.map((region) => (
								<Typography.Mono key={region.id}>
									#{region.id} · {region.members} cells ·{" "}
									{amount(region.strength)} energy · {percent(region.authority)}{" "}
									authority
								</Typography.Mono>
							))}
							{!view?.regions?.length && (
								<Typography.Mono>No evidenced activity yet.</Typography.Mono>
							)}
						</Flex.Column>
					</Section>
					<Section>
						<Section.Header title="Decision journal" meta="persisted" />
						<Section.Body>
							{events.map((event) => (
								<JournalEntry
									key={`${event.lane}-${event.id}-${event.kind}-${event.at}`}
									event={event}
								/>
							))}
						</Section.Body>
					</Section>
				</Flex.Column>
			</Flex>
		</Flex.Column>
	);
};
