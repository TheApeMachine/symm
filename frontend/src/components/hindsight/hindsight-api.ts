import type {
	HindsightExcursion,
	HindsightStream,
	HindsightTick,
} from "#/collections/hindsight";
import { hubBaseUrl } from "#/lib/hub";
import type {
	HindsightEpisode,
	HindsightPolicyEpoch,
	HindsightReferencePoint,
	HindsightRun,
	HindsightSymbolSummary,
	HindsightTimeline,
	HindsightTimelineBucket,
	MarketCoordinate,
	ReferenceRole,
	TimelineAxis,
} from "./hindsight-types";

/*
The run list is the hub's one hindsight REST route; everything else streams
over the hub socket (collections/hindsight.ts) and is projected here, in the
browser, onto the timeline the components draw. Sequences are stored tick
numbers.
*/

export const fetchHindsightRuns = async (): Promise<HindsightRun[]> => {
	const response = await fetch(`${hubBaseUrl()}/hindsight/runs`);

	if (!response.ok) {
		throw new Error(
			`hindsight runs: ${response.status} ${await response.text()}`,
		);
	}

	const runs = (await response.json()) as Array<{
		id: unknown;
		startedAt: unknown;
		status: unknown;
		trained: unknown;
		policies: unknown;
	}>;

	const isPolicyEpoch = (value: unknown): value is HindsightPolicyEpoch => {
		const policy = value as Partial<HindsightPolicyEpoch> | null;
		return (
			typeof policy?.epoch === "string" &&
			typeof policy.gate === "boolean" &&
			typeof policy.surpriseZ === "number" &&
			typeof policy.warmup === "string" &&
			typeof policy.learningRate === "number"
		);
	};

	return runs.map((run) => {
		if (
			typeof run.id !== "string" ||
			typeof run.startedAt !== "string" ||
			!Array.isArray(run.policies) ||
			!run.policies.every(isPolicyEpoch)
		) {
			throw new Error(`hindsight runs: malformed run ${JSON.stringify(run)}`);
		}

		return {
			id: run.id,
			startedAt: run.startedAt,
			status: String(run.status),
			trained: run.trained === true,
			policies: run.policies,
		};
	});
};

const iso = (ms: number) => new Date(ms).toISOString();
const isoNs = (ns: number) => new Date(ns / 1e6).toISOString();

/*
coordinateOf is a tick's price under coordinate; null where the tick does
not define it (a missing touch side is NaN on the wire).
*/
export const coordinateOf = (
	tick: HindsightTick,
	coordinate: MarketCoordinate,
): number | null => {
	const { bid, ask, trade_price: trade } = tick.fields;

	switch (coordinate) {
		case "midpoint":
			return Number.isFinite(bid) && Number.isFinite(ask)
				? (bid + ask) / 2
				: null;
		case "bid":
			return Number.isFinite(bid) ? bid : null;
		case "ask":
			return Number.isFinite(ask) ? ask : null;
		case "trade":
		case "last":
			return Number.isFinite(trade) ? trade : null;
	}
};

const KINDS: Record<string, HindsightEpisode["kind"]> = {
	up: "upward_excursion",
	down: "downward_excursion",
};

/*
episodeOf is a stored up/down excursion as an episode: A..D bound it, B is
the ignition (shock onset), C the exhaustion (peak or trough), and entry and
exit the executable points train priced.
*/
export const episodeOf = (
	symbol: string,
	excursion: HindsightExcursion,
	coordinate: MarketCoordinate,
	ticks: HindsightTick[],
): HindsightEpisode | null => {
	const kind = KINDS[excursion.kind];
	if (kind === undefined) return null;

	const f = excursion.fields;
	const at = (tick: number) => ticks.find((entry) => entry.tick === tick)?.at;
	const point = (
		role: ReferenceRole,
		tick: number,
		value: number,
		venueNs: number,
	): HindsightReferencePoint => {
		const received = at(tick);

		return {
			role,
			capture: {
				run: "",
				sequence: tick,
				stream: "tick",
				streamEpoch: 0,
				streamSequence: tick,
			},
			ordinal: 0,
			venueAt: isoNs(venueNs),
			receivedAt: received === undefined ? "" : iso(received),
			value,
			hasValue: Number.isFinite(value),
		};
	};

	const move = (f.c_price - f.b_price) / f.b_price;
	const inside = ticks.filter(
		(tick) => tick.tick >= f.a_tick && tick.tick <= f.d_tick,
	).length;

	return {
		id: excursion.key,
		symbol,
		kind,
		coordinate,
		fromSequence: f.a_tick,
		toSequence: f.d_tick,
		fromAt: iso(at(f.a_tick) ?? f.b_at / 1e6),
		toAt: iso(at(f.d_tick) ?? f.c_at / 1e6),
		observations: inside,
		observedExcursion: move,
		hasObservedExcursion: Number.isFinite(move),
		confirmed: true,
		ratio: 0,
		hasRatio: false,
		traversed: 0,
		hasTraversed: false,
		threshold: 0,
		hasThreshold: false,
		references: [
			point("anchor", f.entry_tick, f.entry_ask, f.entry_at),
			point("shock_onset", f.b_tick, f.b_price, f.b_at),
			point(
				kind === "upward_excursion" ? "peak" : "trough",
				f.c_tick,
				f.c_price,
				f.c_at,
			),
			point("exit_anchor", f.exit_tick, f.exit_bid, f.exit_at),
		],
	};
};

/*
bucketize splits ticks within [from, to] into at most count buckets by tick
number. A bucket with no tick defining coordinate is not defined: its prices
are not invented.
*/
export const bucketize = (
	ticks: HindsightTick[],
	coordinate: MarketCoordinate,
	count: number,
	from: number,
	to: number,
): HindsightTimelineBucket[] => {
	const width = Math.max(1, Math.ceil((to - from + 1) / count));
	const buckets: HindsightTimelineBucket[] = [];
	// Ticks defining each bucket's spread, for its running mean.
	const spreads: number[] = [];

	for (const tick of ticks) {
		if (tick.tick < from || tick.tick > to) continue;

		const index = Math.floor((tick.tick - from) / width);
		let bucket = buckets[index];

		if (bucket === undefined) {
			const fromSequence = from + index * width;
			bucket = {
				index,
				fromSequence,
				toSequence: Math.min(to, fromSequence + width - 1),
				fromAt: iso(tick.at),
				toAt: iso(tick.at),
				observedFromSequence: tick.tick,
				observedToSequence: tick.tick,
				observedFromAt: iso(tick.at),
				observedToAt: iso(tick.at),
				observations: 0,
				tickers: 0,
				trades: 0,
				tradeQty: 0,
				defined: false,
				open: 0,
				high: 0,
				low: 0,
				close: 0,
				spreadFraction: 0,
				hasSpreadFraction: false,
				touchDepth: 0,
				hasTouchDepth: false,
				captureRate: 0,
				hasCaptureRate: false,
			};
			buckets[index] = bucket;
		}

		bucket.observations++;
		bucket.trades += tick.fields.trades;
		bucket.tradeQty += tick.fields.trade_qty;
		bucket.observedToSequence = tick.tick;
		bucket.observedToAt = iso(tick.at);
		bucket.toAt = iso(tick.at);

		const { bid, ask } = tick.fields;

		if (Number.isFinite(bid) && Number.isFinite(ask) && bid + ask > 0) {
			const spread = (ask - bid) / ((ask + bid) / 2);
			spreads[index] = (spreads[index] ?? 0) + 1;
			bucket.spreadFraction +=
				(spread - bucket.spreadFraction) / spreads[index];
			bucket.hasSpreadFraction = true;
		}

		const price = coordinateOf(tick, coordinate);
		if (price === null) continue;

		if (!bucket.defined) {
			bucket.defined = true;
			bucket.open = bucket.high = bucket.low = price;
		}

		bucket.high = Math.max(bucket.high, price);
		bucket.low = Math.min(bucket.low, price);
		bucket.close = price;
	}

	// No book depth is streamed: touchDepth stays undefined (hasTouchDepth).
	return buckets.filter((bucket) => bucket !== undefined);
};

export const summariesOf = (
	index: HindsightStream | null,
): HindsightSymbolSummary[] =>
	(index?.symbols ?? []).map(({ symbol, fields }) => ({
		symbol,
		observations: fields.ticks,
		defined: fields.touched,
		tickers: 0,
		trades: fields.trades,
		firstSequence: fields.first_tick,
		lastSequence: fields.last_tick,
		firstAt: isoNs(fields.first_at),
		lastAt: isoNs(fields.last_at),
		episodes: fields.up + fields.down,
		insufficientData: fields.ticks < 2,
		topExcursion: Math.abs(fields.top_excursion),
		topKind:
			fields.top_excursion > 0
				? "upward_excursion"
				: fields.top_excursion < 0
					? "downward_excursion"
					: undefined,
		priceEpisodes: fields.up + fields.down,
		regimeEpisodes: 0,
	}));

/*
timelineOf projects a symbol's stream onto count buckets over window (the
whole run when null).
*/
export const timelineOf = (
	stream: HindsightStream,
	index: HindsightStream | null,
	coordinate: MarketCoordinate,
	axis: TimelineAxis,
	count: number,
	window: { from: number; to: number } | null,
): HindsightTimeline | null => {
	const { ticks } = stream;
	if (ticks.length === 0) return null;

	const first = ticks[0];
	const last = ticks[ticks.length - 1];
	const from = window?.from ?? first.tick;
	const to = window?.to ?? last.tick;
	const inWindow = ticks.filter((tick) => tick.tick >= from && tick.tick <= to);
	const episodes = stream.excursions
		.map((excursion) => episodeOf(stream.symbol, excursion, coordinate, ticks))
		.filter((episode): episode is HindsightEpisode => episode !== null);

	return {
		run: stream.run,
		symbol: stream.symbol,
		coordinate,
		axis,
		span: {
			fromSequence: from,
			toSequence: to,
			fromAt: iso(inWindow[0]?.at ?? first.at),
			toAt: iso(inWindow[inWindow.length - 1]?.at ?? last.at),
		},
		runSpan: {
			fromSequence: first.tick,
			toSequence: last.tick,
			fromAt: iso(first.at),
			toAt: iso(last.at),
		},
		buckets: bucketize(ticks, coordinate, count, from, to),
		discovery: {
			symbol: stream.symbol,
			coordinate,
			stored: stream.excursions.length,
			episodes,
			insufficientData: ticks.length < 2,
		},
		streams: [],
		symbols: summariesOf(index),
	};
};
