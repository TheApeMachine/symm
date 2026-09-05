import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	describeEpisode,
	episodeReadout,
	REFERENCE_GLYPHS,
	REFERENCE_MEANING,
} from "./episode-palette";
import type {
	HindsightEpisode,
	HindsightGap,
	HindsightLifecycleEvent,
	HindsightTimeline,
	HindsightTimelineBucket,
} from "./hindsight-types";
import { type Position, positionInstant, positionsFor } from "./positions";
import {
	buildScale,
	buildValueScale,
	formatClock,
	formatCount,
	formatPrice,
	observationGap,
	type TimelineScale,
} from "./timeline-scale";

/*
The Hindsight timeline.

It is one horizontal capture axis read top to bottom:

    episode ribbons     what the declared selector found interesting
    coordinate track    the declared market coordinate, with reference points
    arrivals            how many observations of each kind landed
    microstructure      quoted spread and quoted touch size
    operational         reconnects, capture gaps, and what the desk did

Everything is addressed by CaptureSequence. Clicking anywhere parks the
playhead on a frame that was actually captured, so the inspector below is
always looking at an exact identity rather than at a time neighbourhood.

The operational band is drawn apart from the market bands on purpose. Episode
discovery must be independent of SYMM's trading outputs, so what the desk did
is shown beside the market record and never inside the evidence that selected
it.
*/

const LANE_HEIGHT = 11;
const LANE_GAP = 2;
const TRACK_HEIGHT = 186;
const DENSITY_HEIGHT = 46;
const MICRO_HEIGHT = 30;
const OPERATIONAL_HEIGHT = 26;
const AXIS_HEIGHT = 16;
const OVERVIEW_HEIGHT = 46;

type Band = {
	top: number;
	height: number;
};

type Lanes = {
	geometry: Band;
	regime: Band;
	track: Band;
	density: Band;
	micro: Band;
	operational: Band;
	axis: Band;
	total: number;
};

const layout = (geometryLanes: number, regimeLanes: number): Lanes => {
	const geometryHeight = Math.max(geometryLanes, 1) * (LANE_HEIGHT + LANE_GAP);
	const regimeHeight = Math.max(regimeLanes, 1) * (LANE_HEIGHT + LANE_GAP);

	let cursor = 3;
	const geometry = { top: cursor, height: geometryHeight };
	cursor += geometryHeight + 2;
	const regime = { top: cursor, height: regimeHeight };
	cursor += regimeHeight + 4;
	const track = { top: cursor, height: TRACK_HEIGHT };
	cursor += TRACK_HEIGHT;
	const density = { top: cursor, height: DENSITY_HEIGHT };
	cursor += DENSITY_HEIGHT;
	const micro = { top: cursor, height: MICRO_HEIGHT };
	cursor += MICRO_HEIGHT;
	const operational = { top: cursor, height: OPERATIONAL_HEIGHT };
	cursor += OPERATIONAL_HEIGHT;
	const axis = { top: cursor, height: AXIS_HEIGHT };
	cursor += AXIS_HEIGHT;

	return {
		geometry,
		regime,
		track,
		density,
		micro,
		operational,
		axis,
		total: cursor,
	};
};

/*
useMeasure reports the element's content width so the timeline can be drawn in
real pixels. A stretched viewBox would scale the glyphs and the hairlines along
with the data, which is exactly what makes a dense chart unreadable.
*/
const useMeasure = <T extends HTMLElement>() => {
	const ref = useRef<T | null>(null);
	const [width, setWidth] = useState(0);

	useEffect(() => {
		const element = ref.current;

		if (element === null) return;

		const observer = new ResizeObserver((entries) => {
			for (const entry of entries) {
				setWidth(entry.contentRect.width);
			}
		});

		observer.observe(element);
		setWidth(element.clientWidth);

		return () => observer.disconnect();
	}, []);

	return { ref, width };
};

type PlacedEpisode = {
	episode: HindsightEpisode;
	lane: number;
	left: number;
	right: number;
};

/*
placeEpisodes stacks overlapping ribbons into lanes, largest first, so the
biggest observed geometry keeps the lane nearest the track and a crowd of small
overlapping spans never hides it.
*/
const placeEpisodes = (
	episodes: HindsightEpisode[],
	scale: TimelineScale,
): PlacedEpisode[] => {
	const placed: PlacedEpisode[] = [];
	const laneEnds: number[] = [];

	for (const episode of episodes) {
		const left = scale.xOf(episode.fromSequence);
		const right = Math.max(scale.xOf(episode.toSequence), left + 2);

		let lane = laneEnds.findIndex((end) => end <= left - 3);

		if (lane === -1) {
			lane = laneEnds.length;
			laneEnds.push(right);
		} else {
			laneEnds[lane] = right;
		}

		placed.push({ episode, lane, left, right });
	}

	return placed;
};

export type TimelineProps = {
	timeline: HindsightTimeline | null;
	gaps: HindsightGap[];
	lifecycle: HindsightLifecycleEvent[];
	positions: Position[];
	playhead: number | null;
	marks: number[];
	selectedEpisode: string | null;
	selectedPosition: string | null;
	onPlayhead: (sequence: number) => void;
	onEpisode: (episode: HindsightEpisode) => void;
	onPosition: (position: Position) => void;
	onZoom: (from: number, to: number) => void;
	onResetZoom: () => void;
};

export const Timeline = ({
	timeline,
	gaps,
	lifecycle,
	positions,
	playhead,
	marks,
	selectedEpisode,
	selectedPosition,
	onPlayhead,
	onEpisode,
	onPosition,
	onZoom,
	onResetZoom,
}: TimelineProps) => {
	const { ref, width } = useMeasure<HTMLDivElement>();
	const [hover, setHover] = useState<number | null>(null);
	const [drag, setDrag] = useState<{ from: number; to: number } | null>(null);

	const scale = useMemo(() => buildScale(timeline, width), [timeline, width]);
	const buckets = timeline?.buckets ?? [];
	const episodes = timeline?.discovery.episodes ?? [];

	const geometry = useMemo(
		() =>
			placeEpisodes(
				episodes.filter(
					(episode) => describeEpisode(episode.kind).lane === "geometry",
				),
				scale,
			),
		[episodes, scale],
	);

	const regime = useMemo(
		() =>
			placeEpisodes(
				episodes.filter(
					(episode) => describeEpisode(episode.kind).lane === "regime",
				),
				scale,
			),
		[episodes, scale],
	);

	const lanes = useMemo(
		() =>
			layout(
				geometry.reduce((most, entry) => Math.max(most, entry.lane + 1), 0),
				regime.reduce((most, entry) => Math.max(most, entry.lane + 1), 0),
			),
		[geometry, regime],
	);

	const values = useMemo(
		() => buildValueScale(buckets, lanes.track.height),
		[buckets, lanes.track.height],
	);

	const hoverBucket = hover === null ? null : scale.bucketAt(hover);

	/*
		Desk records are correlated by decision, not by capture, so a position
		edge is placed by the instant the venue reported for its fill. That is a
		rendering coordinate only: the position's identity remains its decision,
		and clicking it parks the playhead on the nearest frame that really was
		captured rather than on an interpolated one.
	*/
	const xOfInstant = useCallback(
		(instant: number | null): number | null => {
			if (instant === null || buckets.length === 0) return null;

			let previous: { at: number; x: number } | null = null;

			for (const bucket of buckets) {
				if (bucket.observations === 0) continue;

				const from = new Date(bucket.observedFromAt).getTime();
				const to = new Date(bucket.observedToAt).getTime();

				if (Number.isNaN(from) || Number.isNaN(to)) continue;

				const left = bucket.index * scale.step;

				if (instant <= to && instant >= from) {
					const span = to - from;

					return left + (span > 0 ? ((instant - from) / span) * scale.step : 0);
				}

				if (instant < from) {
					return previous === null ? left : left;
				}

				previous = { at: to, x: left + scale.step };
			}

			return previous?.x ?? null;
		},
		[buckets, scale],
	);

	const drawn = useMemo(
		() => (timeline === null ? [] : positionsFor(positions, timeline.symbol)),
		[positions, timeline],
	);

	const positionOf = useCallback(
		(event: React.MouseEvent<SVGSVGElement>): number => {
			const bounds = event.currentTarget.getBoundingClientRect();

			return Math.min(Math.max(event.clientX - bounds.left, 0), width);
		},
		[width],
	);

	/*
		The ribbons are painted marks, not controls, so the surface hit-tests them:
		a click landing inside one selects that episode, a click anywhere else parks
		the playhead. Keyboard reach to the same episodes is the Episodes list.
	*/
	const episodeAt = (x: number, y: number): HindsightEpisode | null => {
		const bands: Array<[Band, PlacedEpisode[]]> = [
			[lanes.geometry, geometry],
			[lanes.regime, regime],
		];

		for (const [band, entries] of bands) {
			if (y < band.top || y > band.top + band.height) continue;

			for (const entry of entries) {
				const top = band.top + entry.lane * (LANE_HEIGHT + LANE_GAP);

				if (y < top || y > top + LANE_HEIGHT) continue;
				if (x < entry.left || x > entry.right) continue;

				return entry.episode;
			}
		}

		return null;
	};

	/*
		A position is hit anywhere inside its span on the coordinate track, so
		the whole shaded region is the target rather than a two-pixel marker.
	*/
	const positionAt = (x: number, y: number): Position | null => {
		if (y < lanes.track.top || y > lanes.track.top + lanes.track.height) {
			return null;
		}

		for (const position of drawn) {
			const from = xOfInstant(positionInstant(position.entry));

			if (from === null) continue;

			const to = xOfInstant(positionInstant(position.exit)) ?? from + 6;

			if (x >= from - 4 && x <= Math.max(to, from + 6) + 4) {
				return position;
			}
		}

		return null;
	};

	if (timeline === null || width === 0) {
		return (
			<div ref={ref} className="min-h-40 flex-1">
				<p className="px-4 py-8 font-mono text-[10px] text-(--f4)">
					{timeline === null
						? "Select a run to project its capture tape."
						: "Measuring…"}
				</p>
			</div>
		);
	}

	const empty = buckets.length === 0;

	return (
		<div ref={ref} className="relative w-full select-none">
			{empty ? (
				<p className="px-4 py-8 font-mono text-[10px] text-(--f4)">
					No market observations were captured for{" "}
					{timeline.symbol || "this run"}.
				</p>
			) : (
				<svg
					width={width}
					height={lanes.total}
					role="img"
					aria-label={`Capture timeline for ${timeline.symbol}: ${timeline.coordinate} coordinate, ${episodes.length} episodes`}
					className="block cursor-crosshair"
					onMouseMove={(event) => {
						const x = positionOf(event);
						setHover(x);
						setDrag((current) =>
							current === null ? null : { ...current, to: x },
						);
					}}
					onMouseLeave={() => {
						setHover(null);
						setDrag(null);
					}}
					onMouseDown={(event) => {
						const x = positionOf(event);
						setDrag({ from: x, to: x });
					}}
					onMouseUp={(event) => {
						const x = positionOf(event);
						const bounds = event.currentTarget.getBoundingClientRect();
						const y = event.clientY - bounds.top;
						const hit = episodeAt(x, y);
						const stationary = drag === null || Math.abs(x - drag.from) <= 4;

						if (hit !== null && stationary) {
							onEpisode(hit);
							setDrag(null);
							return;
						}

						const held = stationary ? positionAt(x, y) : null;

						if (held !== null) {
							onPosition(held);
							setDrag(null);
							return;
						}

						if (drag !== null && Math.abs(x - drag.from) > 4) {
							const from = scale.sequenceAt(Math.min(drag.from, x));
							const to = scale.sequenceAt(Math.max(drag.from, x));
							onZoom(Math.floor(from), Math.ceil(to));
						} else {
							onPlayhead(Math.round(scale.sequenceAt(x)));
						}

						setDrag(null);
					}}
					onDoubleClick={onResetZoom}
				>
					<defs>
						<pattern
							id="hindsight-undefined"
							width="4"
							height="4"
							patternTransform="rotate(45)"
							patternUnits="userSpaceOnUse"
						>
							<line
								x1="0"
								y1="0"
								x2="0"
								y2="4"
								stroke="var(--line2)"
								strokeWidth="1.4"
							/>
						</pattern>
						<pattern
							id="hindsight-gap"
							width="5"
							height="5"
							patternTransform="rotate(45)"
							patternUnits="userSpaceOnUse"
						>
							<line
								x1="0"
								y1="0"
								x2="0"
								y2="5"
								stroke="var(--down)"
								strokeWidth="2"
							/>
						</pattern>
					</defs>

					<EpisodeBand
						band={lanes.geometry}
						placed={geometry}
						selected={selectedEpisode}
					/>
					<EpisodeBand
						band={lanes.regime}
						placed={regime}
						selected={selectedEpisode}
					/>

					<CoordinateTrack
						band={lanes.track}
						buckets={buckets}
						values={values}
						scale={scale}
						episodes={episodes}
						width={width}
					/>

					<PositionOverlay
						band={lanes.track}
						positions={drawn}
						values={values}
						xOfInstant={xOfInstant}
						selected={selectedPosition}
						width={width}
					/>

					<ArrivalBand band={lanes.density} buckets={buckets} scale={scale} />
					<MicrostructureBand
						band={lanes.micro}
						buckets={buckets}
						scale={scale}
					/>
					<OperationalBand
						band={lanes.operational}
						timeline={timeline}
						gaps={gaps}
						lifecycle={lifecycle}
						scale={scale}
						width={width}
					/>
					<AxisBand
						band={lanes.axis}
						buckets={buckets}
						timeline={timeline}
						width={width}
					/>

					{drag !== null && Math.abs(drag.to - drag.from) > 4 ? (
						<rect
							x={Math.min(drag.from, drag.to)}
							y={0}
							width={Math.abs(drag.to - drag.from)}
							height={lanes.axis.top}
							fill="var(--acc)"
							opacity={0.14}
							stroke="var(--acc)"
							strokeWidth={1}
						/>
					) : null}

					{hover !== null ? (
						<line
							x1={hover}
							y1={0}
							x2={hover}
							y2={lanes.axis.top}
							stroke="var(--f4)"
							strokeWidth={1}
							strokeDasharray="2 3"
						/>
					) : null}

					{marks.map((mark, index) => (
						<g key={`mark-${mark}`}>
							<line
								x1={scale.xOf(mark)}
								y1={0}
								x2={scale.xOf(mark)}
								y2={lanes.axis.top}
								stroke="var(--info)"
								strokeWidth={1}
								strokeDasharray="4 2"
							/>
							<rect
								x={scale.xOf(mark) - 6}
								y={lanes.track.top - 11}
								width={12}
								height={10}
								fill="var(--info)"
								rx={1}
							/>
							<text
								x={scale.xOf(mark)}
								y={lanes.track.top - 3}
								textAnchor="middle"
								fontSize={7.5}
								fill="var(--bg)"
								className="font-mono"
							>
								{String.fromCharCode(65 + index)}
							</text>
						</g>
					))}

					{playhead !== null ? (
						<g>
							<line
								x1={scale.xOf(playhead)}
								y1={0}
								x2={scale.xOf(playhead)}
								y2={lanes.axis.top}
								stroke="var(--f1)"
								strokeWidth={1}
							/>
							<circle
								cx={scale.xOf(playhead)}
								cy={lanes.track.top + 4}
								r={3}
								fill="var(--f1)"
							/>
						</g>
					) : null}
				</svg>
			)}

			<HoverReadout
				bucket={hoverBucket}
				timeline={timeline}
				coordinate={timeline.coordinate}
			/>
		</div>
	);
};

const EpisodeBand = ({
	band,
	placed,
	selected,
}: {
	band: Band;
	placed: PlacedEpisode[];
	selected: string | null;
}) => (
	<g>
		{placed.map(({ episode, lane, left, right }) => {
			const descriptor = describeEpisode(episode.kind);
			const active = selected === episode.id;
			const y = band.top + lane * (LANE_HEIGHT + LANE_GAP);
			const span = Math.max(right - left, 2);

			return (
				<g key={episode.id} className="cursor-pointer">
					<title>
						{`${descriptor.name} · ${episode.symbol} · ${episodeReadout(episode)}\n` +
							`capture ${episode.fromSequence}–${episode.toSequence} · ${episode.observations} observations` +
							`${episode.confirmed ? "" : " · unconfirmed (tape ends inside it)"}\n${descriptor.meaning}`}
					</title>
					<rect
						x={left}
						y={y}
						width={span}
						height={LANE_HEIGHT}
						fill={descriptor.color}
						opacity={active ? 0.85 : 0.4}
						stroke={descriptor.color}
						strokeWidth={active ? 1.5 : 0.75}
						strokeDasharray={episode.confirmed ? undefined : "3 2"}
						rx={1.5}
					/>
					{span > 42 ? (
						<text
							x={left + 4}
							y={y + LANE_HEIGHT - 3}
							className="pointer-events-none font-mono"
							fontSize={7.5}
							fill="var(--bg)"
							opacity={active ? 1 : 0.85}
						>
							{descriptor.label} {episodeReadout(episode).split(" · ")[0]}
						</text>
					) : null}
				</g>
			);
		})}
	</g>
);

const CoordinateTrack = ({
	band,
	buckets,
	values,
	scale,
	episodes,
	width,
}: {
	band: Band;
	buckets: HindsightTimelineBucket[];
	values: ReturnType<typeof buildValueScale>;
	scale: TimelineScale;
	episodes: HindsightEpisode[];
	width: number;
}) => {
	const centre = (bucket: HindsightTimelineBucket) =>
		(bucket.index + 0.5) * scale.step;

	/*
		The track is broken only where the record was genuinely silent.

		An empty bucket between two observations is usually just the axis
		resolving finer than the instrument was quoted, and breaking the line
		there would shred a perfectly continuous record into confetti. A hole
		many times longer than this instrument's own mean observation interval
		is a different thing: nothing was observed across it, and the coordinate
		there is unavailable rather than flat. Those get a dotted bridge instead
		of a line, so the shape stays readable and the absence stays visible.
	*/
	const mean = observationGap(buckets);
	const silence = mean === null ? Number.POSITIVE_INFINITY : mean * 12;

	const runs: HindsightTimelineBucket[][] = [];
	let run: HindsightTimelineBucket[] = [];
	let last: HindsightTimelineBucket | null = null;

	for (const bucket of buckets) {
		if (!bucket.defined) continue;

		const silent =
			last !== null &&
			new Date(bucket.observedFromAt).getTime() -
				new Date(last.observedToAt).getTime() >
				silence;

		if (silent && run.length > 0) {
			runs.push(run);
			run = [];
		}

		run.push(bucket);
		last = bucket;
	}

	if (run.length > 0) runs.push(run);

	return (
		<g transform={`translate(0, ${band.top})`}>
			<rect
				x={0}
				y={0}
				width={width}
				height={band.height}
				fill="var(--sunken)"
			/>

			{buckets.map((bucket) =>
				bucket.defined || bucket.observations === 0 ? null : (
					<rect
						key={`undefined-${bucket.index}`}
						x={bucket.index * scale.step}
						y={0}
						width={scale.step}
						height={band.height}
						fill="url(#hindsight-undefined)"
						opacity={0.35}
					>
						<title>
							{`${bucket.observations} observations, coordinate undefined here — not zero.`}
						</title>
					</rect>
				),
			)}

			{runs.slice(1).map((segment, index) => {
				const previous = runs[index];
				const last = previous[previous.length - 1];
				const first = segment[0];

				/*
					Nothing was observed between these two runs of the track. The
					connector is drawn dotted rather than solid because SYMM was not
					told what the coordinate did in between — an unobserved span is
					unavailable, not flat.
				*/
				return (
					<line
						key={`bridge-${first.index}`}
						x1={centre(last)}
						y1={values.yOf(last.close)}
						x2={centre(first)}
						y2={values.yOf(first.open)}
						stroke="var(--f4)"
						strokeWidth={1}
						strokeDasharray="1 3"
					/>
				);
			})}

			{runs.map((segment) => {
				const key = `run-${segment[0].index}`;
				const upper = segment
					.map((bucket) => `${centre(bucket)},${values.yOf(bucket.high)}`)
					.join(" L ");
				const lower = segment
					.slice()
					.reverse()
					.map((bucket) => `${centre(bucket)},${values.yOf(bucket.low)}`)
					.join(" L ");
				const line = segment
					.map(
						(bucket, index) =>
							`${index === 0 ? "M" : "L"} ${centre(bucket)},${values.yOf(bucket.close)}`,
					)
					.join(" ");

				return (
					<g key={key}>
						<path
							d={`M ${upper} L ${lower} Z`}
							fill="var(--acc)"
							opacity={0.16}
						/>
						<path d={line} fill="none" stroke="var(--acc)" strokeWidth={1.25} />
					</g>
				);
			})}

			{values.defined ? (
				<g
					className="pointer-events-none font-mono"
					fontSize={8}
					fill="var(--f4)"
				>
					<text x={4} y={11}>
						{formatPrice(values.high)}
					</text>
					<text x={4} y={band.height - 4}>
						{formatPrice(values.low)}
					</text>
				</g>
			) : null}

			{episodes.flatMap((episode) =>
				episode.references.map((reference) => {
					const x = scale.xOf(reference.capture.sequence);
					const y = reference.hasValue
						? values.yOf(reference.value)
						: band.height / 2;
					const descriptor = describeEpisode(episode.kind);

					return (
						<g
							key={`${episode.id}-${reference.role}`}
							className="pointer-events-auto"
						>
							<title>
								{`${reference.role.replace("_", " ")} · capture ${reference.capture.sequence}:${reference.ordinal}\n` +
									`${REFERENCE_MEANING[reference.role]}`}
							</title>
							<line
								x1={x}
								y1={Math.max(y - 9, 0)}
								x2={x}
								y2={Math.min(y + 9, band.height)}
								stroke={descriptor.color}
								strokeWidth={0.75}
								opacity={0.6}
							/>
							<text
								x={x}
								y={y + 3}
								textAnchor="middle"
								fontSize={8}
								fill={descriptor.color}
							>
								{REFERENCE_GLYPHS[reference.role]}
							</text>
						</g>
					);
				}),
			)}
		</g>
	);
};

/*
PositionOverlay draws what the desk actually did, on top of the coordinate the
market actually printed.

The entry and exit sit at the prices the venue reported filling at, not at the
midpoint the track is drawn from — those are different quantities, and drawing
a fill on the midpoint line would quietly claim the desk transacted at a price
it never saw. The span between them is shaded so the relationship between the
holding period and the market's own geometry is readable at a glance, and it is
dashed and tinted apart from the episode ribbons because it is the desk's
record, not the evidence that selected the episode.
*/
const PositionOverlay = ({
	band,
	positions,
	values,
	xOfInstant,
	selected,
	width,
}: {
	band: Band;
	positions: Position[];
	values: ReturnType<typeof buildValueScale>;
	xOfInstant: (instant: number | null) => number | null;
	selected: string | null;
	width: number;
}) => (
	<g transform={`translate(0, ${band.top})`}>
		{positions.map((position) => {
			const from = xOfInstant(positionInstant(position.entry));

			if (from === null) return null;

			const to = xOfInstant(positionInstant(position.exit));
			const right = to ?? width;
			const active = selected === position.decisionId;
			const entryPrice = position.entry?.price ?? null;
			const exitPrice = position.exit?.price ?? null;
			const change = position.realisedPriceChange;
			const tone =
				change === null
					? "var(--f3)"
					: change >= 0
						? "var(--up)"
						: "var(--down)";

			return (
				<g key={position.decisionId} className="cursor-pointer">
					<title>
						{`${position.symbol} · decision ${position.decisionId}\n` +
							`entry ${entryPrice === null ? "unrecorded" : formatPrice(entryPrice)}` +
							` @ ${position.entry?.at ?? "—"}\n` +
							`exit  ${
								position.open
									? "still open at the end of the recorded tape"
									: exitPrice === null
										? "unrecorded"
										: `${formatPrice(exitPrice)} @ ${position.exit?.at ?? "—"}`
							}\n` +
							`${
								change === null
									? "realised price change undefined — both fills were not recorded"
									: `realised price change between fills ${(change * 100).toFixed(2)}%`
							}\n` +
							`${position.fees === null ? "fees unreported" : `fees ${position.fees.toFixed(4)}`}\n` +
							"Positioned by the venue's reported fill instant; its identity is the decision above."}
					</title>

					<rect
						x={from}
						y={0}
						width={Math.max(right - from, 2)}
						height={band.height}
						fill={tone}
						opacity={active ? 0.14 : 0.07}
					/>
					<line
						x1={from}
						y1={0}
						x2={from}
						y2={band.height}
						stroke={tone}
						strokeWidth={active ? 1.5 : 1}
						strokeDasharray="3 2"
					/>
					{to === null ? null : (
						<line
							x1={to}
							y1={0}
							x2={to}
							y2={band.height}
							stroke={tone}
							strokeWidth={active ? 1.5 : 1}
							strokeDasharray="3 2"
						/>
					)}

					{entryPrice === null || !values.defined ? null : (
						<g>
							<line
								x1={from - 5}
								y1={values.yOf(entryPrice)}
								x2={to === null ? right : to}
								y2={values.yOf(entryPrice)}
								stroke={tone}
								strokeWidth={0.75}
								strokeDasharray="1 3"
								opacity={0.8}
							/>
							<path
								d={`M ${from},${values.yOf(entryPrice) - 5} L ${from + 4.5},${values.yOf(entryPrice) + 3} L ${from - 4.5},${values.yOf(entryPrice) + 3} Z`}
								fill={tone}
							/>
						</g>
					)}

					{exitPrice === null || to === null || !values.defined ? null : (
						<path
							d={`M ${to},${values.yOf(exitPrice) + 5} L ${to + 4.5},${values.yOf(exitPrice) - 3} L ${to - 4.5},${values.yOf(exitPrice) - 3} Z`}
							fill={tone}
						/>
					)}

					{right - from > 46 ? (
						<text
							x={from + 4}
							y={band.height - 4}
							className="pointer-events-none font-mono"
							fontSize={7.5}
							fill={tone}
							opacity={active ? 1 : 0.8}
						>
							{position.open
								? "HELD"
								: change === null
									? "HELD · undefined"
									: `HELD ${change >= 0 ? "+" : ""}${(change * 100).toFixed(2)}%`}
						</text>
					) : null}
				</g>
			);
		})}
	</g>
);

const ArrivalBand = ({
	band,
	buckets,
	scale,
}: {
	band: Band;
	buckets: HindsightTimelineBucket[];
	scale: TimelineScale;
}) => {
	const peak = buckets.reduce(
		(most, bucket) => Math.max(most, bucket.observations),
		1,
	);
	const usable = band.height - 8;

	return (
		<g transform={`translate(0, ${band.top})`}>
			<text
				x={4}
				y={9}
				className="pointer-events-none font-mono"
				fontSize={7}
				fill="var(--f4)"
			>
				ARRIVALS · peak {formatCount(peak)}
			</text>
			{buckets.map((bucket) => {
				const x = bucket.index * scale.step;
				const trades = (bucket.trades / peak) * usable;
				const tickers = (bucket.tickers / peak) * usable;

				return (
					<g key={`arrivals-${bucket.index}`}>
						<title>
							{`${bucket.tickers} ticker · ${bucket.trades} trade observations\n` +
								`capture ${bucket.observedFromSequence}–${bucket.observedToSequence}`}
						</title>
						<rect
							x={x}
							y={band.height - tickers - trades}
							width={Math.max(scale.step - 0.6, 0.6)}
							height={tickers}
							fill="var(--info)"
							opacity={0.5}
						/>
						<rect
							x={x}
							y={band.height - trades}
							width={Math.max(scale.step - 0.6, 0.6)}
							height={trades}
							fill="var(--acc)"
							opacity={0.7}
						/>
					</g>
				);
			})}
		</g>
	);
};

const MicrostructureBand = ({
	band,
	buckets,
	scale,
}: {
	band: Band;
	buckets: HindsightTimelineBucket[];
	scale: TimelineScale;
}) => {
	const peakSpread = buckets.reduce(
		(most, bucket) =>
			bucket.hasSpreadFraction ? Math.max(most, bucket.spreadFraction) : most,
		0,
	);
	const peakDepth = buckets.reduce(
		(most, bucket) =>
			bucket.hasTouchDepth ? Math.max(most, bucket.touchDepth) : most,
		0,
	);
	const half = band.height / 2;

	return (
		<g transform={`translate(0, ${band.top})`}>
			<text
				x={4}
				y={8}
				className="pointer-events-none font-mono"
				fontSize={7}
				fill="var(--f4)"
			>
				QUOTED SPREAD / TOUCH SIZE
			</text>
			{buckets.map((bucket) => {
				const x = bucket.index * scale.step;
				const barWidth = Math.max(scale.step - 0.6, 0.6);
				const spread =
					bucket.hasSpreadFraction && peakSpread > 0
						? (bucket.spreadFraction / peakSpread) * (half - 2)
						: 0;
				const depth =
					bucket.hasTouchDepth && peakDepth > 0
						? (bucket.touchDepth / peakDepth) * (half - 2)
						: 0;

				return (
					<g key={`micro-${bucket.index}`}>
						<title>
							{`${bucket.hasSpreadFraction ? `spread ${(bucket.spreadFraction * 100).toFixed(3)}% of midpoint` : "spread unavailable"}\n` +
								`${bucket.hasTouchDepth ? `quoted touch size ${bucket.touchDepth.toPrecision(4)}` : "touch size unavailable"}`}
						</title>
						<rect
							x={x}
							y={half - spread}
							width={barWidth}
							height={spread}
							fill="var(--info)"
							opacity={0.55}
						/>
						<rect
							x={x}
							y={half}
							width={barWidth}
							height={depth}
							fill="var(--brand)"
							opacity={0.4}
						/>
					</g>
				);
			})}
			<line
				x1={0}
				y1={half}
				x2={scale.width}
				y2={half}
				stroke="var(--line)"
				strokeWidth={0.5}
			/>
		</g>
	);
};

const OperationalBand = ({
	band,
	timeline,
	gaps,
	lifecycle,
	scale,
	width,
}: {
	band: Band;
	timeline: HindsightTimeline;
	gaps: HindsightGap[];
	lifecycle: HindsightLifecycleEvent[];
	scale: TimelineScale;
	width: number;
}) => {
	const from = timeline.span.fromSequence;
	const to = timeline.span.toSequence;
	const inWindow = (sequence: number) => sequence >= from && sequence <= to;

	return (
		<g transform={`translate(0, ${band.top})`}>
			<rect
				x={0}
				y={0}
				width={width}
				height={band.height}
				fill="var(--surface)"
			/>
			<text
				x={4}
				y={9}
				className="pointer-events-none font-mono"
				fontSize={7}
				fill="var(--f4)"
			>
				OPERATIONAL · what the process did, kept apart from the market record
			</text>

			{timeline.streams
				.filter((stream) => stream.reconnect && inWindow(stream.fromSequence))
				.map((stream) => {
					const x = scale.xOf(stream.fromSequence);

					return (
						<g key={`${stream.stream}-${stream.epoch}`}>
							<title>
								{`reconnect · ${stream.stream} entered epoch ${stream.epoch} at capture ${stream.fromSequence}\n` +
									"A new stream epoch is a new connection: frames before and after it are distinguishable by identity, not by time."}
							</title>
							<line
								x1={x}
								y1={11}
								x2={x}
								y2={band.height}
								stroke="var(--warn)"
								strokeWidth={1}
								strokeDasharray="2 2"
							/>
							<text
								x={x + 2}
								y={band.height - 2}
								fontSize={7}
								fill="var(--warn)"
							>
								↻{stream.epoch}
							</text>
						</g>
					);
				})}

			{gaps
				.filter((gap) => gap.sequence > 0 && inWindow(gap.sequence))
				.map((gap) => (
					<g key={`${gap.encoding}-${gap.sequence}-${gap.detail}`}>
						<title>
							{`capture integrity defect · ${gap.encoding} at sequence ${gap.sequence}\n${gap.detail}\n` +
								"Inspection certainty is broken across this interval."}
						</title>
						<rect
							x={scale.xOf(gap.sequence) - 2}
							y={11}
							width={4}
							height={band.height - 12}
							fill="url(#hindsight-gap)"
						/>
					</g>
				))}

			{lifecycle.map((event) => {
				const at = new Date(event.at).getTime();

				if (Number.isNaN(at)) return null;

				// Lifecycle transitions happen inside the broker after a decision
				// committed, so they are correlated by decision, not by capture. They
				// are positioned by their recorded instant and labelled as such.
				const bucket = timeline.buckets.find((candidate) => {
					const start = new Date(candidate.observedFromAt).getTime();
					const end = new Date(candidate.observedToAt).getTime();

					return at >= start && at <= end;
				});

				if (bucket === undefined) return null;

				const x = (bucket.index + 0.5) * scale.step;
				const exit =
					event.kind.includes("exit") || event.kind.includes("close");

				return (
					<g key={`${event.decisionId}-${event.kind}-${event.at}`}>
						<title>
							{`${event.kind}${event.action ? ` (${event.action})` : ""} · ${event.symbol}\n` +
								`decision ${event.decisionId} · ${event.at}\n` +
								"Positioned by recorded instant: a lifecycle transition is correlated by decision, not by capture identity."}
						</title>
						<circle
							cx={x}
							cy={band.height - 7}
							r={3}
							fill={exit ? "var(--down)" : "var(--up)"}
							stroke="var(--bg)"
							strokeWidth={0.75}
						/>
					</g>
				);
			})}
		</g>
	);
};

const AxisBand = ({
	band,
	buckets,
	timeline,
	width,
}: {
	band: Band;
	buckets: HindsightTimelineBucket[];
	timeline: HindsightTimeline;
	width: number;
}) => {
	const ticks = 6;
	const step = width / ticks;

	return (
		<g transform={`translate(0, ${band.top})`}>
			<line
				x1={0}
				y1={0}
				x2={width}
				y2={0}
				stroke="var(--line)"
				strokeWidth={1}
			/>
			{Array.from({ length: ticks + 1 }, (_, index) => index).map((index) => {
				const x = Math.min(index * step, width - 1);
				const bucket =
					buckets[
						Math.min(
							buckets.length - 1,
							Math.floor((index / ticks) * buckets.length),
						)
					];

				if (bucket === undefined) return null;

				const label =
					timeline.axis === "time"
						? formatClock(bucket.fromAt || bucket.observedFromAt)
						: `#${bucket.fromSequence || bucket.observedFromSequence}`;

				return (
					<g key={`tick-${x}`}>
						<line
							x1={x}
							y1={0}
							x2={x}
							y2={3}
							stroke="var(--line2)"
							strokeWidth={1}
						/>
						<text
							x={index === ticks ? x - 2 : x + 2}
							y={11}
							textAnchor={index === ticks ? "end" : "start"}
							className="font-mono"
							fontSize={7.5}
							fill="var(--f4)"
						>
							{label}
						</text>
					</g>
				);
			})}
		</g>
	);
};

const HoverReadout = ({
	bucket,
	timeline,
	coordinate,
}: {
	bucket: HindsightTimelineBucket | null;
	timeline: HindsightTimeline;
	coordinate: string;
}) => (
	<div className="flex flex-wrap items-center gap-x-4 gap-y-0.5 border-(--line) border-t px-3 py-1.5 font-mono text-[9px] text-(--f4)">
		{bucket === null ? (
			<span>
				drag to zoom · click to park the playhead · double-click to fit the run
			</span>
		) : (
			<>
				<span>
					capture{" "}
					<span className="text-(--f1) tabular-nums">
						{bucket.observedFromSequence || "—"}–
						{bucket.observedToSequence || "—"}
					</span>
				</span>
				<span>
					{timeline.axis === "time" ? "at" : "clock"}{" "}
					<span className="text-(--f2)">
						{formatClock(bucket.observedFromAt || bucket.fromAt)}
					</span>
				</span>
				<span>
					{coordinate}{" "}
					{bucket.defined ? (
						<span className="text-(--f1) tabular-nums">
							{formatPrice(bucket.open)} → {formatPrice(bucket.close)}
						</span>
					) : (
						<span className="text-(--warn)">undefined</span>
					)}
				</span>
				<span>
					obs{" "}
					<span className="text-(--f2) tabular-nums">
						{bucket.tickers}t / {bucket.trades}x
					</span>
				</span>
				{bucket.hasSpreadFraction ? (
					<span>
						spread{" "}
						<span className="text-(--f2) tabular-nums">
							{(bucket.spreadFraction * 100).toFixed(3)}%
						</span>
					</span>
				) : null}
				{bucket.hasCaptureRate ? (
					<span>
						observer{" "}
						<span className="text-(--f2) tabular-nums">
							{bucket.captureRate.toFixed(0)} captures/s
						</span>
					</span>
				) : null}
			</>
		)}
	</div>
);

/*
Overview is the whole run under the detail view, with the plotted window drawn
on it. It answers "where am I in the tape?" without a scrollbar's worth of
guesswork, and dragging on it moves the window.
*/
export const Overview = ({
	timeline,
	window,
	onZoom,
	onResetZoom,
}: {
	timeline: HindsightTimeline | null;
	window: { from: number; to: number } | null;
	onZoom: (from: number, to: number) => void;
	onResetZoom: () => void;
}) => {
	const { ref, width } = useMeasure<HTMLDivElement>();
	const [drag, setDrag] = useState<{ from: number; to: number } | null>(null);
	const scale = useMemo(() => buildScale(timeline, width), [timeline, width]);
	const buckets = timeline?.buckets ?? [];
	const values = useMemo(
		() => buildValueScale(buckets, OVERVIEW_HEIGHT - 12, 3),
		[buckets],
	);

	if (timeline === null || width === 0 || buckets.length === 0) {
		return <div ref={ref} className="h-12 w-full" />;
	}

	/*
		React clears currentTarget once the handler returns, so the pointer
		position must be read synchronously and only the resulting number may
		travel into a state updater — an updater runs later, by which time the
		event has nothing left to measure against.
	*/
	const positionOf = (event: React.MouseEvent<SVGSVGElement>): number => {
		const bounds = event.currentTarget.getBoundingClientRect();

		return Math.min(Math.max(event.clientX - bounds.left, 0), width);
	};

	const line = buckets
		.filter((bucket) => bucket.defined)
		.map(
			(bucket, index) =>
				`${index === 0 ? "M" : "L"} ${(bucket.index + 0.5) * scale.step},${values.yOf(bucket.close) + 8}`,
		)
		.join(" ");

	return (
		<div ref={ref} className="w-full select-none">
			<svg
				width={width}
				height={OVERVIEW_HEIGHT}
				role="img"
				aria-label={`Whole-run overview for ${timeline.symbol} with the plotted window marked`}
				className="block cursor-ew-resize"
				onMouseDown={(event) => {
					const x = positionOf(event);
					setDrag({ from: x, to: x });
				}}
				onMouseMove={(event) => {
					const x = positionOf(event);

					setDrag((current) =>
						current === null ? null : { ...current, to: x },
					);
				}}
				onMouseLeave={() => setDrag(null)}
				onMouseUp={(event) => {
					const x = positionOf(event);

					if (drag !== null && Math.abs(x - drag.from) > 4) {
						onZoom(
							Math.floor(scale.sequenceAt(Math.min(drag.from, x))),
							Math.ceil(scale.sequenceAt(Math.max(drag.from, x))),
						);
					}

					setDrag(null);
				}}
				onDoubleClick={onResetZoom}
			>
				<rect
					x={0}
					y={0}
					width={width}
					height={OVERVIEW_HEIGHT}
					fill="var(--sunken)"
				/>
				<path d={line} fill="none" stroke="var(--f3)" strokeWidth={1} />

				{timeline.discovery.episodes.map((episode) => {
					const descriptor = describeEpisode(episode.kind);

					if (descriptor.lane !== "geometry") return null;

					const left = scale.xOf(episode.fromSequence);
					const right = Math.max(scale.xOf(episode.toSequence), left + 1.5);

					return (
						<rect
							key={`overview-${episode.id}`}
							x={left}
							y={OVERVIEW_HEIGHT - 4}
							width={right - left}
							height={3}
							fill={descriptor.color}
							opacity={0.8}
						/>
					);
				})}

				{window !== null ? (
					<g>
						<rect
							x={0}
							y={0}
							width={scale.xOf(window.from)}
							height={OVERVIEW_HEIGHT}
							fill="var(--bg)"
							opacity={0.6}
						/>
						<rect
							x={scale.xOf(window.to)}
							y={0}
							width={Math.max(width - scale.xOf(window.to), 0)}
							height={OVERVIEW_HEIGHT}
							fill="var(--bg)"
							opacity={0.6}
						/>
						<rect
							x={scale.xOf(window.from)}
							y={0}
							width={Math.max(scale.xOf(window.to) - scale.xOf(window.from), 1)}
							height={OVERVIEW_HEIGHT}
							fill="none"
							stroke="var(--acc)"
							strokeWidth={1}
						/>
					</g>
				) : null}

				{drag !== null && Math.abs(drag.to - drag.from) > 4 ? (
					<rect
						x={Math.min(drag.from, drag.to)}
						y={0}
						width={Math.abs(drag.to - drag.from)}
						height={OVERVIEW_HEIGHT}
						fill="var(--acc)"
						opacity={0.18}
					/>
				) : null}
			</svg>
		</div>
	);
};
