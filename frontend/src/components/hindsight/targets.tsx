import { useMemo, useState } from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Input } from "#/components/ui/input";
import { Section } from "#/components/ui/section";
import {
	describeEpisode,
	episodeRank,
	episodeReadout,
	REFERENCE_GLYPHS,
} from "./episode-palette";
import type {
	HindsightEpisode,
	HindsightSymbolSummary,
	HindsightTimeline,
} from "./hindsight-types";
import { formatCount } from "./timeline-scale";

/*
The inspection targets.

Instruments are ranked by the distance the declared coordinate travelled, which
is market geometry and nothing else — no Opportunity, no Perspective, no
decision took part in choosing what appears at the top. A high rank says the
market moved there. It says nothing about what SYMM should have done there.
*/

export const SymbolTargets = ({
	summaries,
	selected,
	onSelect,
}: {
	summaries: HindsightSymbolSummary[];
	selected: string | null;
	onSelect: (symbol: string) => void;
}) => {
	const [filter, setFilter] = useState("");
	const [onlyMoved, setOnlyMoved] = useState(true);

	const rows = useMemo(() => {
		const needle = filter.trim().toUpperCase();

		return summaries.filter((summary) => {
			if (needle !== "" && !summary.symbol.toUpperCase().includes(needle)) {
				return false;
			}

			return onlyMoved ? summary.priceEpisodes > 0 : true;
		});
	}, [summaries, filter, onlyMoved]);

	const peak = rows.reduce((most, row) => Math.max(most, row.topExcursion), 0);

	return (
		<Section fit="pane" surface="surface" className="min-h-0 flex-1">
			<Section.Header
				title="Targets"
				size="m"
				rule
				sticky
				meta={
					<span className="font-mono text-[9px] text-(--f4)">
						{formatCount(rows.length)}/{formatCount(summaries.length)}
					</span>
				}
			/>
			<div className="shrink-0 border-(--line) border-b px-2.5 py-2">
				<Input
					value={filter}
					placeholder="filter instrument"
					className="w-full font-mono text-[10px]"
					onChange={(event) => setFilter(event.currentTarget.value)}
				/>
				<Button
					variant="bare"
					size="xs"
					title="Show only instruments where the declared coordinate travelled far enough to qualify under the selector."
					className="mt-1.5 w-full justify-start gap-1 whitespace-nowrap px-0 font-mono text-[9px] text-(--f4) hover:text-(--f2)"
					onClick={() => setOnlyMoved((current) => !current)}
				>
					<span className={onlyMoved ? "text-(--acc)" : ""}>
						{onlyMoved ? "▣" : "▢"}
					</span>
					only instruments that moved
				</Button>
			</div>
			<Section.Body>
				{rows.length === 0 ? (
					<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
						No instrument on this tape qualified under the declared selector.
					</p>
				) : null}
				<ul className="flex flex-col divide-y divide-(--line)">
					{rows.map((summary) => {
						const active = selected === summary.symbol;
						const share = peak > 0 ? summary.topExcursion / peak : 0;
						const descriptor =
							summary.topKind === undefined ? null : describeEpisode(summary.topKind);

						return (
							<li key={summary.symbol}>
								<Button
									variant="bare"
									className={`relative flex w-full flex-col items-start gap-0.5 px-2.5 py-1.5 text-left hover:bg-(--raised) ${
										active ? "bg-(--raised)" : ""
									}`}
									onClick={() => onSelect(summary.symbol)}
								>
									<span
										aria-hidden
										className="absolute inset-y-0 left-0 opacity-15"
										style={{
											width: `${(share * 100).toFixed(1)}%`,
											background: descriptor?.color ?? "var(--f4)",
										}}
									/>
									<Flex.Row
										align="center"
										justify="between"
										className="relative w-full"
									>
										<span className="truncate font-mono text-[10px] font-semibold text-(--f1)">
											{summary.symbol}
										</span>
										<span
											className="font-mono text-[10px] tabular-nums"
											style={{ color: descriptor?.color ?? "var(--f4)" }}
										>
											{summary.topExcursion > 0
												? `${(summary.topExcursion * 100).toFixed(2)}%`
												: "—"}
										</span>
									</Flex.Row>
									<Flex.Row
										align="center"
										gap={2}
										className="relative w-full font-mono text-[8px] text-(--f4)"
									>
										<span
											title="Price-geometry episodes / microstructure-regime episodes"
											className="tabular-nums"
										>
											{summary.priceEpisodes} price · {summary.regimeEpisodes} regime
										</span>
										<span>·</span>
										<span
											className="tabular-nums"
											title="Observations where the declared coordinate was defined, of all observations captured."
										>
											{formatCount(summary.defined)}/{formatCount(summary.observations)}
										</span>
										{summary.insufficientData ? (
											<span className="text-(--warn)">· short</span>
										) : null}
									</Flex.Row>
								</Button>
							</li>
						);
					})}
				</ul>
			</Section.Body>
		</Section>
	);
};

/*
The episode list for the selected instrument: the concrete moments this session
can put under the microscope, each resolving to the exact captures its
reference points name.
*/
export const EpisodeTargets = ({
	timeline,
	selected,
	onSelect,
	onReference,
}: {
	timeline: HindsightTimeline | null;
	selected: string | null;
	onSelect: (episode: HindsightEpisode) => void;
	onReference: (sequence: number) => void;
}) => {
	const episodes = useMemo(
		() =>
			[...(timeline?.discovery.episodes ?? [])].sort(
				(left, right) => episodeRank(right) - episodeRank(left),
			),
		[timeline],
	);

	const discovery = timeline?.discovery;

	return (
		<Section fit="pane" surface="surface" className="min-h-0 flex-1">
			<Section.Header
				title="Episodes"
				size="m"
				rule
				sticky
				meta={
					<span className="font-mono text-[9px] text-(--f4)">
						{episodes.length} found
					</span>
				}
			/>
			{discovery !== undefined ? (
				<div className="shrink-0 border-(--line) border-b bg-(--sunken) px-2.5 py-1.5 font-mono text-[8px] text-(--f4) leading-relaxed">
					<div>
						selector: {discovery.coordinate} · qualifying move{" "}
						<span className="text-(--f2)">
							{(discovery.qualifyingMove * 100).toFixed(3)}%
						</span>{" "}
						= max(floor {(discovery.policy.floorExcursion * 100).toFixed(2)}%,{" "}
						{discovery.policy.excursionSigmas}σ ×{" "}
						{discovery.hasSigma ? discovery.sigma.toExponential(2) : "σ undefined"} × √
						{discovery.policy.excursionHorizon})
					</div>
					<div>
						measured on{" "}
						<span className="text-(--f2)">{formatCount(discovery.defined)}</span> of{" "}
						{formatCount(discovery.observations)} observations ·{" "}
						<span className="text-(--warn)">{formatCount(discovery.undefined)}</span>{" "}
						undefined (not zero)
					</div>
				</div>
			) : null}
			<Section.Body>
				{episodes.length === 0 ? (
					<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
						{discovery?.insufficientData
							? "Too few defined observations here for the selector to answer. Reported as unmeasured, not as quiet."
							: "Nothing on this instrument's tape qualified under the declared selector."}
					</p>
				) : null}
				<ul className="flex flex-col divide-y divide-(--line)">
					{episodes.map((episode) => {
						const descriptor = describeEpisode(episode.kind);
						const active = selected === episode.id;

						return (
							<li key={episode.id}>
								<Button
									variant="bare"
									className={`flex w-full flex-col items-start gap-1 px-2.5 py-2 text-left hover:bg-(--raised) ${
										active ? "bg-(--raised)" : ""
									}`}
									onClick={() => onSelect(episode)}
								>
									<Flex.Row align="center" justify="between" className="w-full">
										<Flex.Row align="center" gap={2}>
											<span
												aria-hidden
												className="inline-block h-2 w-2 rounded-[1px]"
												style={{ background: descriptor.color }}
											/>
											<span className="font-mono text-[10px] font-semibold text-(--f1)">
												{descriptor.name}
											</span>
											{episode.confirmed ? null : (
												<span
													className="font-mono text-[8px] text-(--warn)"
													title="The tape ends inside this episode: its close has not been observed."
												>
													unconfirmed
												</span>
											)}
										</Flex.Row>
										<span
											className="font-mono text-[10px] tabular-nums"
											style={{ color: descriptor.color }}
										>
											{episodeReadout(episode)}
										</span>
									</Flex.Row>
									<span className="font-mono text-[8px] text-(--f4)">
										capture {episode.fromSequence}–{episode.toSequence} ·{" "}
										{episode.observations} observations · {episode.coordinate}
									</span>
									<Flex.Row gap={1} className="flex-wrap">
										{episode.references.map((reference) => (
											<span
												key={reference.role}
												className="cursor-pointer rounded-[2px] border border-(--line2) px-1 py-0.5 font-mono text-[8px] text-(--f3) hover:border-(--acc) hover:text-(--f1)"
												onClickCapture={(event) => {
													event.stopPropagation();
													onReference(reference.capture.sequence);
												}}
											>
												{REFERENCE_GLYPHS[reference.role]} {reference.role.replace("_", " ")}{" "}
												<span className="tabular-nums text-(--f4)">
													#{reference.capture.sequence}
												</span>
											</span>
										))}
									</Flex.Row>
								</Button>
							</li>
						);
					})}
				</ul>
			</Section.Body>
		</Section>
	);
};
