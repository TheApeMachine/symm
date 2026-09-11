import { useState } from "react";
import { Alert } from "#/components/ui/alert";
import { Badge } from "#/components/ui/badge";
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
import { Explain } from "./explain";
import { action, amount, basis, clock, percent } from "./format";
import { KnowledgePanel } from "./knowledge-panel";
import { ImpulseMap } from "./map";
import { LearningPerformanceBanner } from "./performance-banner";
import { RecognitionPanel } from "./recognition-panel";
import { RehearsalPanel } from "./rehearsal-panel";
import { SkillPanel } from "./skill-panel";
import { type LearningEvent, type Region, useLearning } from "./state";
import { LearningVisualizer } from "./visualizer";

const JournalEntry = ({ event }: { event: LearningEvent }) => (
	<Flex.Row
		align="center"
		gap={2}
		className="border-(--line) border-b px-3 py-1.5"
	>
		<Typography.Mono size="s" tone="f4" className="shrink-0">
			{clock(event.at)}
		</Typography.Mono>
		<Typography.Mono size="s" tone="f2" className="min-w-0 flex-1 truncate">
			{action(event.action, event.power, event.reduce)}
		</Typography.Mono>
		<Typography.Mono
			size="s"
			tone={event.kind === "resolved" ? "accent" : "f3"}
			className="shrink-0"
			title={`${event.mode} ${event.lane + 1} · ${event.kind}`}
		>
			{event.kind === "resolved"
				? basis(event.target ?? 0)
				: amount(event.profit)}
		</Typography.Mono>
	</Flex.Row>
);

const hottest = (regions: Region[] | null, region: Region) => {
	const strongest = Math.max(...(regions ?? []).map((entry) => entry.strength), 0);

	return strongest > 0 ? (region.strength / strongest) * 100 : 0;
};

type Tab =
	| "decision"
	| "recognition"
	| "capital"
	| "influence"
	| "forward"
	| "wallets";

const TABS: Array<{ key: Tab; label: string }> = [
	{ key: "decision", label: "Main agent decision" },
	{ key: "recognition", label: "Precursor recognition" },
	{ key: "capital", label: "Main agent economics" },
	{ key: "influence", label: "Precursor discovery" },
	{ key: "forward", label: "Forward test" },
	{ key: "wallets", label: "Agent channels" },
];

export const LearningDashboard = () => {
	const [symbol, setSymbol] = useState("");
	const [tab, setTab] = useState<Tab>("recognition");
	const { view, events, error } = useLearning(symbol);

	return (
		<Flex.Column className="h-full min-h-0 w-full">
			<Section.Header
				title="Precursor recognition"
				meta={
					view
						? view.status === "reading the record"
							? `${view.status} · ${Number(view.rehearsal?.observations ?? 0).toLocaleString()} of ${Number(view.rehearsal?.budget ?? 0).toLocaleString()} captured observations`
							: `${view.steps.toLocaleString()} frames · ${view.decisions.toLocaleString()} learned situations · ${view.columns} quantities`
						: "Connecting to the workspace"
				}
			/>
			{error && <Alert>{error} · Last successful state remains visible.</Alert>}
			<RehearsalPanel view={view} />
			<LearningPerformanceBanner view={view} />
			<Flex className="min-h-0 flex-1 max-lg:flex-col">
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
						meta={error || view?.status}
					>
						<Badge
							label={
								error
									? "offline"
									: view?.skill?.mode === "trading"
										? `trading · ${view.skill.account}`
										: "learning"
							}
							variant={
								error
									? "error"
									: view?.skill?.mode !== "trading"
										? "info"
										: view.skill.account === "real"
											? "error"
											: "success"
							}
							dot
						/>
						<Typography.Mono size="s" tone="f3" className="truncate">
							{view?.regions?.length ?? 0} hot regions · grid v
							{view?.gridVersion ?? 0}
						</Typography.Mono>
					</Section.Header>
					{/*
						The band is resizable rather than a fixed height. It is the part
						of the surface worth the most space, but it shares one scroller
						with the panels underneath: pinned tall it left them a sliver to
						scroll inside, and pinned short it wasted the screen. The reader
						decides, and the browser's own resize handle does it without a
						drag implementation of this surface's own.
					*/}
					<div className="flex h-100 max-h-[80vh] min-h-40 shrink-0 resize-y overflow-hidden border-(--line) border-b max-2xl:h-auto max-2xl:resize-none max-2xl:flex-col">
						<div className="w-100 shrink-0 border-(--line) border-r max-2xl:h-85 max-2xl:w-full max-2xl:border-r-0 max-2xl:border-b">
							<ImpulseMap
								points={view?.points ?? []}
								regions={view?.regions ?? []}
								className="h-full w-full"
							/>
						</div>
						<div className="min-w-0 flex-1 bg-(--surface) max-2xl:h-85">
							<LearningVisualizer
								view={view}
								events={events}
								className="h-full w-full"
							/>
						</div>
					</div>

					{/*
						Sticky, because it is the control for everything below it: once
						the panels are long enough to scroll, a tab strip that scrolled
						away with them left no way back without returning to the top.
					*/}
					<Flex.Row
						gap={2}
						className="sticky top-0 z-2 shrink-0 border-(--line) border-b bg-(--surface) px-3 py-2"
					>
						<Tabs size="m" className="flex-wrap">
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
					{tab === "recognition" && <RecognitionPanel />}
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
						<Section.Header title="Hot regions" meta="strongest first">
							<Explain>
								A region is a community of numeric cells the tape lights up
								together. Bar length is its energy against the strongest region
								currently lit; authority is how much of the map's evidence
								stands behind it.
							</Explain>
						</Section.Header>
						<Flex.Column className="gap-1.5 p-3">
							{view?.regions?.map((region) => (
								<Flex.Row key={region.id} align="center" gap={2}>
									<Typography.Mono
										size="s"
										tone="f3"
										className="w-16 shrink-0"
										title={`${region.members} cells`}
									>
										#{region.id}
									</Typography.Mono>
									<div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
										<div
											className="h-full bg-(--acc)"
											style={{ width: `${hottest(view.regions, region)}%` }}
										/>
									</div>
									<Typography.Mono
										size="s"
										tone="f4"
										className="w-24 shrink-0 text-right"
										title={`${amount(region.strength)} energy · ${percent(region.authority)} authority`}
									>
										{percent(region.authority)}
									</Typography.Mono>
								</Flex.Row>
							))}
							{!view?.regions?.length && (
								<Typography.Mono size="s" tone="f3">
									No evidenced activity yet.
								</Typography.Mono>
							)}
						</Flex.Column>
					</Section>
					<Section>
						<Section.Header title="Recent activity" meta="live history">
							<Explain>
								One row per recorded moment, newest last: the time, the call the
								agent made, and what it came to — a tape benefit in basis points
								once the record has answered it, the wallet's own change before
								that.
							</Explain>
						</Section.Header>
						<Section.Body scroll={false}>
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
