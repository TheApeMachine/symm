import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { learningStore } from "#/collections/learning";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { RehearsalTracks } from "./charts";
import { Explain } from "./explain";
import { TrieModal } from "./trie-modal";
import type { LearningView } from "./state";

/*
The pool a worker practises from, by what each mounted fragment's own
coordinate did between its ignition and its extremum.

"Rise eaten by costs" is deliberately not a class here. Separating a rise that
clears its fees from one that does not needs a friction figure this path never
receives, and a fragment shown in that class on no evidence would be the one
reading on this surface that is an opinion rather than a measurement.
*/
const CLASSES = [
	{ key: "profitable", label: "Price rises", colour: "var(--up)" },
	{ key: "declining", label: "Price falls", colour: "var(--down)" },
	{ key: "quiet", label: "No price development", colour: "var(--f3)" },
	{ key: "illiquid", label: "Never quoted", colour: "var(--warn)" },
] as const;

export const RehearsalPanel = ({ view }: { view: LearningView | null }) => {
	const replay = view?.rehearsal;
	// Which learner's memory is open, by its own id rather than by lane
	// position: the lanes are one per universe and their order is the
	// cohort's, not something a reader should have to track.
	const [opened, setOpened] = useState<number | null>(null);
	const learners = useSelector(
		learningStore,
		(state) => state?.recognition?.learners ?? null,
	);
	if (!replay) {
		return (
			<Section fit="content" className="shrink-0 border-(--line) border-b">
				<Section.Header title="Historical tape → precursor recognition" />
				<Section.Body className="p-3">
					<Typography.Mono size="s">
						No rehearsal frame has arrived. The live account is not this tape.
					</Typography.Mono>
				</Section.Body>
			</Section>
		);
	}
	const fragments = Number(replay.episodes);
	const formedAt = Number(replay.warming);
	const formed = view?.status === "recognising precursors" || formedAt > 0;
	const counts = CLASSES.map((entry) => ({
		...entry,
		count: Number(replay[entry.key]),
	}));
	const largest = Math.max(...counts.map((entry) => entry.count), 0);

	return (
		<Section fit="content" className="shrink-0 border-(--line) border-b">
			<Section.Header
				title="Historical tape → precursor recognition"
				meta={String(replay.status)}
			/>
			<Flex className="items-stretch gap-4 p-3 max-xl:flex-col">
				<Flex.Column className="min-w-0 flex-1 gap-2">
					<Flex.Row align="center" gap={1}>
						<Typography.Mono size="s" tone="f2">
							{fragments.toLocaleString()} mounted fragments · {replay.workers}{" "}
							learners
						</Typography.Mono>
						<Explain>
							Every learner is shown the whole mounted pool. A class with no
							fragments cannot be practised yet — it is missing from the record
							read so far, not judged unimportant.
						</Explain>
					</Flex.Row>
					{counts.map((entry) => (
						<Flex.Row key={entry.key} align="center" gap={2}>
							<Typography.Mono
								size="s"
								tone="f3"
								className="w-40 shrink-0 truncate"
							>
								{entry.label}
							</Typography.Mono>
							<div
								className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-[3px] bg-(--line)"
								role="img"
								aria-label={`${entry.label}: ${entry.count} mounted`}
							>
								<div
									className="h-full transition-[width] duration-500"
									style={{
										width: `${largest > 0 ? (entry.count / largest) * 100 : 0}%`,
										background: entry.colour,
									}}
								/>
							</div>
							<Typography.Mono
								size="s"
								tone={entry.count > 0 ? "f2" : "f4"}
								className="w-14 shrink-0 text-right"
							>
								{entry.count > 0 ? entry.count.toLocaleString() : "absent"}
							</Typography.Mono>
						</Flex.Row>
					))}
					<Typography.Mono size="s" tone="f4">
						{replay.runs} runs read ·{" "}
						{Number(replay.observations).toLocaleString()} of{" "}
						{Number(replay.budget).toLocaleString()} observations resident
					</Typography.Mono>
					{/*
						A run the record could not supply is reported here rather than
						becoming the learning phase. The walk carried on without it, so
						the surface stays up and says what it missed.
					*/}
					{Number(replay.skipped) > 0 && (
						<Flex.Row align="center" gap={1}>
							<Typography.Mono size="s" tone="f3">
								{Number(replay.skipped)} runs skipped
							</Typography.Mono>
							<Explain>
								{`The record held these runs but could not supply them, so the walk went on without them. Most recently: ${String(replay.lastSkip ?? "no reason given")}`}
							</Explain>
						</Flex.Row>
					)}
					<Typography.Mono size="s" tone="f4">
						{formed
							? formedAt > 0
								? `Impulse map formed after ${formedAt.toLocaleString()} frames`
								: "Impulse map formed"
							: "Impulse map still forming"}
						{replay.lastSymbol
							? ` · showing ${String(replay.lastSymbol)}${replay.lastAction ? ` · ${String(replay.lastAction)}` : ""}`
							: ""}
					</Typography.Mono>
				</Flex.Column>
				<Flex.Column className="min-w-0 flex-[2] gap-2 border-(--line) border-l pl-4 max-xl:border-l-0 max-xl:pl-0">
					<Flex.Row align="center" gap={1}>
						<Typography.Mono size="s" tone="f2">
							{replay.workers} learners · the tape each is replaying
						</Typography.Mono>
						<Explain>
							The tape runs past a fixed head. B is the observation the move
							ignited at and C the one it ended at; the span between them is the
							move, shaded by what the record says it did. An arrow is a call the
							worker made, coloured by how well it named the moment it reached
							for.
						</Explain>
					</Flex.Row>
					<RehearsalTracks tracks={replay.tracks} onOpen={setOpened} />
					<TrieModal
						open={opened !== null}
						onClose={() => setOpened(null)}
						learner={
							learners?.find((held) => held?.id === opened) ?? null
						}
					/>
				</Flex.Column>
			</Flex>
		</Section>
	);
};
