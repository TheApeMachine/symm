import { useSelector } from "@tanstack/react-store";
import { learningStore } from "#/collections/learning";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Meter } from "#/components/ui/meter";
import { Section } from "#/components/ui/section";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";
import type { LearningAnswerT } from "#/providers/telemetry/telemetry/learning-answer";
import type { LearningLearnerT } from "#/providers/telemetry/telemetry/learning-learner";
import type { LearningRecognitionT } from "#/providers/telemetry/telemetry/learning-recognition";

/*
Nothing here is derived. Every number is what the learner reported, and where it
reported nothing the panel says so rather than showing a zero: a learner that
has not answered and a learner that answered zero are opposite readings.
*/

const decimal = (value: number, places = 3) =>
	Number.isFinite(value) ? value.toFixed(places) : "unavailable";

/*
A winner at zero contrast has not chosen: it is tied with whatever came second.
Saying so is the difference between a reading and a claim.
*/
const Answer = ({ answer }: { answer: LearningAnswerT }) => {
	const asked = String(answer.asked ?? "");
	const answered = String(answer.answered ?? "");
	const runnerUp = String(answer.runnerUp ?? "");
	const tied = answer.contrast === 0;
	const matched = answered === asked;

	return (
		<Flex.Row className="items-center gap-3 border-(--line) border-b px-3 py-2">
			<Typography.Mono size="s" className="w-40 shrink-0">
				shown {asked || "nothing"}
			</Typography.Mono>
			<Badge
				variant={matched && !tied ? "success" : tied ? "disabled" : "error"}
				label={answered || "no association"}
			/>
			<Typography.Mono size="s" className="opacity-70">
				{tied
					? `tied with ${runnerUp || "nothing"}`
					: `ahead of ${runnerUp || "nothing"} by ${decimal(answer.contrast)} bits`}
			</Typography.Mono>
			<Flex.Row className="ml-auto gap-4">
				<Typography.Mono size="s">
					confidence {decimal(answer.confidence)}
				</Typography.Mono>
				<Typography.Mono size="s">
					spread {decimal(answer.ambiguity)}
				</Typography.Mono>
				<Typography.Mono size="s">
					evidence {String(answer.support ?? 0)}
				</Typography.Mono>
			</Flex.Row>
		</Flex.Row>
	);
};

const Learner = ({ learner }: { learner: LearningLearnerT }) => (
	<Section fit="content">
		<Section.Header
			title={`Learner ${learner.id}`}
			meta={`${learner.links} learned situations`}
		/>
		<Section.Body className="p-3">
			<Flex.Row className="flex-wrap gap-2">
				{learner.moments?.length ? (
					learner.moments.map((moment) => (
						<Badge
							key={String(moment?.name)}
							variant="info"
							label={`${String(moment?.name)} · ${moment?.links ?? 0}`}
						/>
					))
				) : (
					<Typography.Mono size="s">
						Nothing recognised yet
					</Typography.Mono>
				)}
			</Flex.Row>
		</Section.Body>
		{learner.answers?.map((answer) =>
			answer ? (
				<Answer key={String(answer.asked)} answer={answer} />
			) : null,
		)}
	</Section>
);

const Grid = ({ recognition }: { recognition: LearningRecognitionT }) => {
	const grid = recognition.grid;
	const regions = grid?.regions ?? [];
	const strongest = regions.reduce(
		(most, region) => Math.max(most, region?.strength ?? 0),
		0,
	);

	return (
		<Section fit="content">
			<Section.Header
				title="Impulse map"
				meta={
					grid?.formed
						? `${String(grid.symbol ?? "")} · ${regions.length} regions lit`
						: "Still settling its layout"
				}
			/>
			<Section.Body className="p-3">
				<Flex.Row className="gap-6">
					<Stat label="Quantities" value={String(grid?.columns ?? 0)} />
					<Stat label="Regions" value={String(regions.length)} />
					<Stat
						label="Fragments"
						value={String(recognition.fragments ?? 0)}
					/>
					<Stat label="Frames" value={String(recognition.frames ?? 0)} />
				</Flex.Row>
			</Section.Body>
			{regions.map((region) =>
				region ? (
					<Flex.Row
						className="items-center gap-3 border-(--line) border-b px-3 py-2"
						key={String(region.id)}
					>
						<Typography.Mono size="s" className="w-24 shrink-0">
							region {String(region.id)}
						</Typography.Mono>
						<Typography.Mono size="s" className="w-28 shrink-0">
							{region.members} members
						</Typography.Mono>
						<Meter
							percent={
								strongest > 0 ? (region.strength / strongest) * 100 : 0
							}
							layout="bar"
							className="min-w-40 flex-1"
						/>
						<Typography.Mono size="s">
							strength {decimal(region.strength, 4)} · authority{" "}
							{decimal(region.authority, 4)}
						</Typography.Mono>
					</Flex.Row>
				) : null,
			)}
		</Section>
	);
};

/*
RecognitionPanel is the whole precursor learner in one view: the map it is being
shown, and for each learner what it holds and what it answers when asked about a
situation it has really seen.
*/
export const RecognitionPanel = () => {
	const state = useSelector(learningStore, (held) => held);
	const recognition = state?.recognition;

	if (!recognition) {
		return (
			<Section fit="content">
				<Section.Header
					title="Precursor recognition"
					meta="No learning frame has arrived yet"
				/>
			</Section>
		);
	}

	return (
		<Flex.Column className="min-h-0 gap-3 overflow-auto p-3">
			<Typography.Mono size="s" className="opacity-70">
				{String(state?.status ?? "")}
			</Typography.Mono>
			<Grid recognition={recognition} />
			{recognition.learners?.map((learner) =>
				learner ? <Learner key={String(learner.id)} learner={learner} /> : null,
			)}
		</Flex.Column>
	);
};
