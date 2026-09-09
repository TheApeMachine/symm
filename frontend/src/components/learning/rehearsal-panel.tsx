import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { RehearsalTracks } from "./charts";
import type { LearningView } from "./state";

export const RehearsalPanel = ({ view }: { view: LearningView | null }) => {
	const replay = view?.rehearsal;
	if (!replay) {
		return (
			<Section fit="content">
				<Section.Header title="Historical practice → live forward test" />
				<Section.Body className="p-3">
					<Typography.Mono>
						Historical workers have not reported. Live account results remain
						separate.
					</Typography.Mono>
				</Section.Body>
			</Section>
		);
	}
	const classes = [
		{
			label: "Rise clears costs",
			count: Number(replay.profitable),
			color: "var(--up)",
		},
		{
			label: "Rise eaten by costs",
			count: Number(replay.subfriction),
			color: "var(--warn)",
		},
		{
			label: "Price falls",
			count: Number(replay.declining),
			color: "var(--down)",
		},
		{
			label: "Exit liquidity unavailable at the touch",
			count: Number(replay.illiquid),
			color: "var(--warn)",
		},
		{
			label: "No price development",
			count: Number(replay.quiet),
			color: "var(--f3)",
		},
	];
	const largest = Math.max(...classes.map((entry) => entry.count));
	const present = classes.filter((entry) => entry.count > 0).length;
	const perClass = present > 0 ? Number(replay.perWorker) / present : 0;

	return (
		<Section fit="content" className="shrink-0 border-(--line) border-b">
			<Section.Header
				title="Historical practice → shared policy → live forward test"
				meta={String(replay.status)}
			/>
			<Flex className="items-stretch gap-4 p-4 max-lg:flex-col">
				<Flex.Column className="min-w-0 flex-1 gap-3">
					<Typography.Mono>Captured episodes · current pool</Typography.Mono>
					{classes.map((entry) => (
						<Flex.Column key={entry.label} className="gap-1">
							<Typography.Mono>
								{entry.label} · {entry.count || "absent"}
							</Typography.Mono>
							<div
								className="h-2 overflow-hidden rounded bg-(--line)"
								role="img"
								aria-label={`${entry.label}: ${entry.count} available, ${entry.count > 0 ? perClass : 0} selected per worker`}
							>
								<div
									className="h-full"
									style={{
										width: `${largest > 0 ? (entry.count / largest) * 100 : 0}%`,
										background: entry.color,
									}}
								/>
							</div>
						</Flex.Column>
					))}
					<Typography.Mono tone="f3">
						Each worker draws {perClass} from each available class, shuffled
						without repeats within its pass.
					</Typography.Mono>
					{present < classes.length && (
						<Typography.Mono>
							Incomplete variety: missing classes cannot be practised yet.
						</Typography.Mono>
					)}
					{replay.ungraded > 0n && (
						<Typography.Mono>
							{String(replay.ungraded)} excluded: missing episode references,
							quotes or fee data.
						</Typography.Mono>
					)}
				</Flex.Column>
				<Flex.Column className="min-w-0 flex-[2] gap-3 border-(--line) border-l pl-4 max-lg:border-l-0 max-lg:pl-0">
					<Typography.Mono>
						{replay.workers} historical workers · the tape each is replaying
					</Typography.Mono>
					<RehearsalTracks tracks={replay.tracks} />
				</Flex.Column>
			</Flex>
		</Section>
	);
};
