import { useState } from "react";
import { Alert } from "#/components/ui/alert";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";
import { CandidateReview } from "./candidate-review";
import { CapitalPanel } from "./capital-panel";
import {
	CandidatePanel,
	DeskPanel,
	ForwardPanel,
	ImpulsePanel,
	InfluencePanel,
	LanePanel,
} from "./decision-panel";
import { action, amount, basis, clock, duration, percent } from "./format";
import { KnowledgePanel } from "./knowledge-panel";
import { ImpulseMap } from "./map";
import { SkillPanel } from "./skill-panel";
import { type LearningEvent, useLearning } from "./state";
import { LearningVisualizer } from "./visualizer";

const JournalEntry = ({event}: {event: LearningEvent}) => <Flex.Column className="gap-1 border-(--line) border-b p-3">
 <Typography.Mono>{clock(event.at)} · {event.mode} {event.lane + 1} · {event.kind}</Typography.Mono>
 <Typography.Mono>{action(event.action, event.power, event.reduce)}</Typography.Mono>
 <Typography.Mono>{event.kind === "resolved" ? `Tape benefit ${basis(event.target ?? 0)}` : `Wallet P&L ${amount(event.profit)}`}</Typography.Mono>
</Flex.Column>;

type Tab = "decision" | "capital" | "influence" | "forward" | "wallets";

const TABS: Array<{ key: Tab; label: string }> = [
	{ key: "decision", label: "Decision" },
	{ key: "capital", label: "Consolidated account" },
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
			{error && <Alert>{error} · Last successful state remains visible.</Alert>}
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
					{/*
						The mode badge and the horizon line sit in the header rather than
						in a strip beneath it: they qualify the title — which symbol, in
						what state, measured over what — and a second full-width band to
						carry three facts cost more vertical space than the panels below
						could spare.
					*/}
					<Section.Header
						title={view?.symbol || "Waiting for market observations"}
						meta={view?.status}
					>
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
						<Typography.Mono size="s" tone="f3" className="truncate">
							Window {duration(view?.horizonNs ?? 0)}
							{view?.horizonCapped ? " (at ceiling)" : ""} ·{" "}
							{view?.epochs?.toLocaleString() ?? 0} impulse epochs observed ·
							grid v{view?.gridVersion ?? 0}
						</Typography.Mono>
					</Section.Header>
					<div className="flex min-h-[340px] border-(--line) border-b max-xl:flex-col">
						<div className="w-[380px] shrink-0 border-(--line) border-r max-xl:w-full max-xl:border-r-0 max-xl:border-b">
							<ImpulseMap
								points={view?.points ?? []}
								regions={view?.regions ?? []}
								className="h-full w-full min-h-[340px]"
							/>
						</div>
						<div className="min-w-0 flex-1 bg-(--surface)">
							<LearningVisualizer
								view={view}
								events={events}
								className="h-full w-full min-h-[340px]"
							/>
						</div>
					</div>

					<Flex.Row gap={2} className="border-(--line) border-b px-3 py-2">
						<Tabs size="m">
							{TABS.map((entry) => (
								<Tabs.Tab
									key={entry.key}
									size="m"
									active={tab === entry.key}
									onClick={() => setTab(entry.key)}
								>
									{entry.label}
								</Tabs.Tab>
							))}
						</Tabs>
					</Flex.Row>

					{tab === "decision" && (
						<>
							<ImpulsePanel view={view} />
							<CandidatePanel view={view} />
							<KnowledgePanel view={view} />
						</>
					)}
					{tab === "influence" && <InfluencePanel view={view} />}
					{tab === "forward" && (
						<>
							<ForwardPanel view={view} />
							<CandidateReview view={view} />
						</>
					)}
					{tab === "wallets" && (
						<>
							<DeskPanel view={view} />
							<LanePanel view={view} />
						</>
					)}
					{tab === "capital" && <CapitalPanel view={view} />}
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
						<Section.Header title="Recent learning activity" meta="live display history" />
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
