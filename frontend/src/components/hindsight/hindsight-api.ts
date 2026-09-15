import * as flatbuffers from "flatbuffers";
import { hubBaseUrl, hubWsUrl } from "#/lib/hub";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import type {
	EpisodeKind,
	HindsightCapture,
	HindsightEnvelope,
	HindsightEpisode,
	HindsightGap,
	HindsightLifecycleEvent,
	HindsightMetricMap,
	HindsightRun,
	HindsightState,
	HindsightSymbolSummary,
	HindsightTimeline,
	HindsightTimelineBucket,
	HindsightTimelineQuery,
	HindsightTimelineSpan,
	MarketCoordinate,
	Measurement,
	ReferenceRole,
} from "./hindsight-types";

export const fetchHindsightRuns = async (): Promise<HindsightRun[]> => {
	const response = await fetch(`${hubBaseUrl()}/hindsight/runs`);

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	const rawRuns = await response.json();

	if (rawRuns === null || rawRuns === undefined) {
		return [];
	}

	if (!Array.isArray(rawRuns)) {
		throw new Error("Hindsight returned invalid run metadata");
	}

	const runs: HindsightRun[] = rawRuns.map((r: any) => {
		if (r && typeof r.id === "string" && r.id !== "") {
			return r as HindsightRun;
		}
		if (r && (typeof r.epoch === "number" || typeof r.epoch === "string")) {
			return {
				...r,
				id: String(r.epoch),
			} as HindsightRun;
		}
		return r as HindsightRun;
	});

	for (const run of runs) {
		if (
			typeof run?.id !== "string" ||
			run.id === "" ||
			typeof run.startedAt !== "string"
		) {
			throw new Error("Hindsight returned invalid run metadata");
		}
		// The date formatter must never receive an unparseable timestamp.
		new Date(run.startedAt).toISOString();
	}

	const seen = new Set<string>();
	const uniqueRuns: HindsightRun[] = [];

	for (const run of runs) {
		if (seen.has(run.id)) {
			continue;
		}

		seen.add(run.id);
		uniqueRuns.push(run);
	}

	return uniqueRuns;
};

export const fetchHindsightSymbols = async (
	run: string,
): Promise<string[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/symbols?run=${encodeURIComponent(run)}`,
	);

	if (response.status === 404) return [];

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as string[];
};

export const fetchHindsightCaptures = async (
	run: string,
	after = 0,
): Promise<HindsightCapture[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/captures?run=${encodeURIComponent(run)}&after=${after}`,
	);

	if (response.status === 404) return [];

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightCapture[];
};

export const fetchHindsightStates = async (
	run: string,
): Promise<HindsightState[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/states?run=${encodeURIComponent(run)}`,
	);

	if (response.status === 404) return [];

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightState[];
};

export const fetchHindsightState = async (
	run: string,
	sequence: number,
	ordinal: number,
): Promise<HindsightState | null> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/state?run=${encodeURIComponent(run)}&seq=${sequence}&ordinal=${ordinal}`,
	);

	if (response.status === 404) return null;

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	const state = (await response.json()) as HindsightState;

	return state.payload ? state : null;
};

export const fetchHindsightEnvelope = async (
	run: string,
	sequence: number,
): Promise<HindsightEnvelope | null> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/envelope?run=${encodeURIComponent(run)}&seq=${sequence}`,
	);

	if (response.status === 404) return null;

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightEnvelope;
};

export const fetchHindsightGaps = async (
	run: string,
): Promise<HindsightGap[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/gaps?run=${encodeURIComponent(run)}`,
	);

	if (response.status === 404) return [];

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightGap[];
};

export const fetchHindsightLifecycle = async (
	run: string,
): Promise<HindsightLifecycleEvent[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/lifecycle?run=${encodeURIComponent(run)}`,
	);

	if (response.status === 404) return [];

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightLifecycleEvent[];
};

export type RawExcursionRecord = {
	epoch: number;
	id: string;
	symbol: string;
	direction: string;
	clears_friction: boolean;
	precursor_start_tick: number;
	anchor_tick: number;
	extremum_tick: number;
	exit_tick: number;
	post_end_tick: number;
	entry_price: number;
	extremum_price: number;
	exit_price: number;
	position_size: number;
	fee: number;
	profit: number;
	profit_fraction: number;
	gross_excursion: number;
	observation_count: number;
	status: string;
};

export const fetchHindsightExcursions = async (
	run: string,
): Promise<RawExcursionRecord[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/excursions?run=${encodeURIComponent(run)}`,
	);

	if (!response.ok) {
		return [];
	}

	return (await response.json()) as RawExcursionRecord[];
};

export const adaptMeasurementsToTimeline = (
	measurements: Measurement[],
	query: HindsightTimelineQuery,
	symbols: string[] = [],
	excursions: RawExcursionRecord[] = [],
): HindsightTimeline => {
	const numBuckets = query.buckets && query.buckets > 0 ? query.buckets : 200;
	const selectedSymbol =
		query.symbol || measurements[0]?.label || measurements[0]?.symbol || "BTC/USD";

	const filtered = measurements.filter(
		(m) =>
			!query.symbol ||
			m.label === query.symbol ||
			m.symbol === query.symbol ||
			m.source.includes("ticker"),
	);

	const totalObservations = measurements.reduce(
		(acc, m) => acc + 1 + (m.peers?.length || 0),
		0,
	);

	const firstSeq =
		filtered[0]?.seqIdx != null
			? Number(filtered[0].seqIdx)
			: filtered[0]?.tick != null
				? Number(filtered[0].tick)
				: 0;
	const lastSeq =
		filtered[filtered.length - 1]?.seqIdx != null
			? Number(filtered[filtered.length - 1].seqIdx)
			: filtered[filtered.length - 1]?.tick != null
				? Number(filtered[filtered.length - 1].tick)
				: firstSeq;
	const firstAt = filtered[0]?.at
		? new Date(filtered[0].at).toISOString()
		: new Date().toISOString();
	const lastAt = filtered[filtered.length - 1]?.at
		? new Date(filtered[filtered.length - 1].at).toISOString()
		: firstAt;

	const span: HindsightTimelineSpan = {
		fromSequence: firstSeq,
		toSequence: lastSeq,
		fromAt: firstAt,
		toAt: lastAt,
	};

	const bucketSize = Math.max(1, Math.ceil(filtered.length / numBuckets));
	const buckets: HindsightTimelineBucket[] = [];

	for (let i = 0; i < filtered.length; i += bucketSize) {
		const chunk = filtered.slice(i, i + bucketSize);
		if (chunk.length === 0) continue;

		let open = 0;
		let high = -Infinity;
		let low = Infinity;
		let close = 0;
		let trades = 0;
		let tradeQty = 0;

		chunk.forEach((m, idx) => {
			const metrics = m.metrics as Record<string, { raw: number }> | undefined;
			const price =
				metrics?.last?.raw ??
				metrics?.price?.raw ??
				((metrics?.bid?.raw ?? 0) + (metrics?.ask?.raw ?? 0)) / 2;

			if (idx === 0) open = price;
			if (price > high) high = price;
			if (price < low) low = price;
			close = price;

			if (m.peers) {
				m.peers.forEach((p) => {
					if (p.source === "spot_trade") {
						trades += 1;
						const peerMetrics = p.metrics as Record<string, { raw: number }> | undefined;
						tradeQty += peerMetrics?.qty?.raw ?? 0;
					}
				});
			}
		});

		if (high === -Infinity) high = 0;
		if (low === Infinity) low = 0;

		const chunkFirstSeq =
			chunk[0]?.seqIdx != null ? Number(chunk[0].seqIdx) : 0;
		const chunkLastSeq =
			chunk[chunk.length - 1]?.seqIdx != null
				? Number(chunk[chunk.length - 1].seqIdx)
				: chunkFirstSeq;
		const chunkFirstAt = chunk[0]?.at
			? new Date(chunk[0].at).toISOString()
			: firstAt;
		const chunkLastAt = chunk[chunk.length - 1]?.at
			? new Date(chunk[chunk.length - 1].at).toISOString()
			: chunkFirstAt;

		const obsCount = chunk.reduce(
			(acc, m) => acc + 1 + (m.peers?.length || 0),
			0,
		);

		buckets.push({
			index: buckets.length,
			fromSequence: chunkFirstSeq,
			toSequence: chunkLastSeq,
			fromAt: chunkFirstAt,
			toAt: chunkLastAt,
			observedFromSequence: chunkFirstSeq,
			observedToSequence: chunkLastSeq,
			observedFromAt: chunkFirstAt,
			observedToAt: chunkLastAt,
			observations: obsCount,
			tickers: chunk.length,
			trades,
			tradeQty,
			defined: true,
			open,
			high,
			low,
			close,
			spreadFraction: 0,
			hasSpreadFraction: false,
			touchDepth: 0,
			hasTouchDepth: false,
			captureRate: 0,
			hasCaptureRate: false,
		});
	}

	const selectedSymbolExcursions = excursions.filter(
		(e) => e.symbol === selectedSymbol,
	);

	const episodes: HindsightEpisode[] = selectedSymbolExcursions.map((e) => ({
		id: e.id,
		symbol: e.symbol,
		kind:
			e.direction === "upward"
				? "upward_excursion"
				: e.direction === "downward"
					? "downward_excursion"
					: "reversal",
		coordinate: "last" as MarketCoordinate,
		fromSequence: e.anchor_tick,
		toSequence: e.exit_tick,
		fromAt: "",
		toAt: "",
		observations: e.observation_count,
		observedExcursion: e.gross_excursion,
		hasObservedExcursion: true,
		confirmed: e.clears_friction,
		ratio: e.profit_fraction,
		hasRatio: true,
		traversed: e.profit_fraction,
		hasTraversed: true,
		threshold: 0.0052,
		hasThreshold: true,
		references: [
			{
				role: "anchor" as ReferenceRole,
				capture: {
					run: String(e.epoch),
					sequence: e.anchor_tick,
					stream: "excursions",
					streamEpoch: e.epoch,
					streamSequence: e.anchor_tick,
				},
				ordinal: e.anchor_tick,
				venueAt: "",
				receivedAt: "",
				value: e.entry_price,
				hasValue: true,
			},
			{
				role: "peak" as ReferenceRole,
				capture: {
					run: String(e.epoch),
					sequence: e.extremum_tick,
					stream: "excursions",
					streamEpoch: e.epoch,
					streamSequence: e.extremum_tick,
				},
				ordinal: e.extremum_tick,
				venueAt: "",
				receivedAt: "",
				value: e.extremum_price,
				hasValue: true,
			},
			{
				role: "exit_anchor" as ReferenceRole,
				capture: {
					run: String(e.epoch),
					sequence: e.exit_tick,
					stream: "excursions",
					streamEpoch: e.epoch,
					streamSequence: e.exit_tick,
				},
				ordinal: e.exit_tick,
				venueAt: "",
				receivedAt: "",
				value: e.exit_price,
				hasValue: true,
			},
		],
	}));

	const allSymbols = symbols.length > 0 ? symbols : [selectedSymbol];
	const symbolSummaries: HindsightSymbolSummary[] = allSymbols.map((sym) => {
		const symExcursions = excursions.filter((e) => e.symbol === sym);
		const topExcursion = symExcursions.reduce(
			(max, e) => Math.max(max, e.gross_excursion),
			0,
		);
		const topKind =
			symExcursions.length > 0
				? symExcursions[0].direction === "upward"
					? ("upward_excursion" as EpisodeKind)
					: ("downward_excursion" as EpisodeKind)
				: undefined;

		return {
			symbol: sym,
			observations: sym === selectedSymbol ? totalObservations : 0,
			defined: sym === selectedSymbol ? filtered.length : 0,
			tickers: sym === selectedSymbol ? filtered.length : 0,
			trades: 0,
			firstSequence: firstSeq,
			lastSequence: lastSeq,
			firstAt,
			lastAt,
			episodes: symExcursions.length,
			insufficientData: false,
			topExcursion,
			topKind,
			priceEpisodes: symExcursions.length,
			regimeEpisodes: 0,
		};
	});

	return {
		run: query.run,
		symbol: selectedSymbol,
		coordinate: query.coordinate || "midpoint",
		policy: {
			coordinate: query.coordinate || "midpoint",
			floorExcursion: 0,
			excursionSigmas: 0,
			excursionHorizon: 0,
			retraceFraction: 0,
			regimeWindow: 0,
			regimeBaseline: 0,
			volatilityRatio: 0,
			spreadRatio: 0,
			depthRatio: 0,
			arrivalRatio: 0,
			minRegimeSpan: 0,
			minObservations: 0,
			maxEpisodesPerSet: 0,
		},
		axis: query.axis || "time",
		span,
		runSpan: span,
		buckets,
		discovery: {
			symbol: selectedSymbol,
			coordinate: query.coordinate || "midpoint",
			policy: {
				coordinate: query.coordinate || "midpoint",
				floorExcursion: 0,
				excursionSigmas: 0,
				excursionHorizon: 0,
				retraceFraction: 0,
				regimeWindow: 0,
				regimeBaseline: 0,
				volatilityRatio: 0,
				spreadRatio: 0,
				depthRatio: 0,
				arrivalRatio: 0,
				minRegimeSpan: 0,
				minObservations: 0,
				maxEpisodesPerSet: 0,
			},
			observations: totalObservations,
			defined: filtered.length,
			undefined: 0,
			sigma: 0,
			hasSigma: false,
			qualifyingMove: 0,
			episodes,
			insufficientData: filtered.length === 0,
		},
		streams: [],
		symbols: symbolSummaries,
		totalObservations,
		totalSymbols: symbolSummaries.length,
		indexedAt: new Date().toISOString(),
		measurements: filtered,
	};
};

const toStr = (val: string | Uint8Array | null | undefined): string => {
	if (!val) return "";
	if (typeof val === "string") return val;
	return new TextDecoder().decode(val);
};

export const flatbufferToMeasurement = (m: MeasurementT): Measurement => {
	const metricsRecord: Record<
		string,
		{
			label?: string;
			raw: number;
			normalized?: number | null;
			unit?: string;
		}
	> = {};

	if (m.metrics && m.metrics.length > 0) {
		for (const metric of m.metrics) {
			const name = toStr(metric?.name);

			if (name) {
				metricsRecord[name] = {
					label: name,
					raw: metric.raw,
					normalized: metric.hasNormalized ? metric.normalized : null,
					unit: toStr(metric.unit) || undefined,
				};
			}
		}
	}

	const metadataRecord: Record<string, string | number> = {};
	if (m.metadata && m.metadata.length > 0) {
		for (const item of m.metadata) {
			const name = toStr(item?.name);

			if (name) {
				metadataRecord[name] = item.value;
			}
		}
	}

	const provenanceRecord: Record<string, string> = {};
	if (m.provenance && m.provenance.length > 0) {
		for (const item of m.provenance) {
			const name = toStr(item?.name);
			const value = toStr(item?.value);

			if (name && value) {
				provenanceRecord[name] = value;
			}
		}
	}

	const atIso =
		m.at > 0n
			? new Date(Number(m.at / 1_000_000n)).toISOString()
			: new Date().toISOString();

	const fromIso =
		m.observedFrom > 0n
			? new Date(Number(m.observedFrom / 1_000_000n)).toISOString()
			: atIso;

	const symbolStr = toStr(m.symbol);
	const sourceStr = toStr(m.source);
	const idStr = toStr(m.id);

	let peersList: Measurement[] | undefined;
	if (m.peers && m.peers.length > 0) {
		peersList = m.peers.map(flatbufferToMeasurement);
	}

	return {
		id: idStr || undefined,
		label: symbolStr,
		symbol: symbolStr,
		source: sourceStr,
		seqIdx: Number(m.tick),
		tick: Number(m.tick),
		at: atIso,
		from: fromIso,
		observedFrom: fromIso,
		maturity: m.maturity,
		snr: m.snr,
		snrDefined: m.snrDefined,
		metrics: metricsRecord,
		metadata: metadataRecord,
		provenance: provenanceRecord,
		peers: peersList,
		Peers: peersList,
	};
};

export type TimelineStreamOptions = {
	signal?: AbortSignal;
	onProgress?: (timeline: HindsightTimeline) => void;
};

/*
fetchHindsightTimeline streams timeline measurements over WebSocket as FlatBuffers
and adapts them directly into a HindsightTimeline.
*/
export const fetchHindsightTimeline = async (
	query: HindsightTimelineQuery,
	options?: TimelineStreamOptions,
): Promise<HindsightTimeline | null> => {
	const params = new URLSearchParams({ run: query.run });

	if (query.symbol) params.set("symbol", query.symbol);
	if (query.coordinate) params.set("coordinate", query.coordinate);
	if (query.axis) params.set("axis", query.axis);
	if (query.buckets) params.set("buckets", String(query.buckets));
	if (query.from) params.set("from", String(query.from));
	if (query.to) params.set("to", String(query.to));

	let symbols: string[] = [];
	if (query.symbols) {
		try {
			symbols = await fetchHindsightSymbols(query.run);
		} catch {
			// optional discovery
		}
	}

	let excursions: RawExcursionRecord[] = [];
	try {
		excursions = await fetchHindsightExcursions(query.run);
	} catch {
		// optional discovery
	}

	return new Promise((resolve, reject) => {
		if (options?.signal?.aborted) {
			reject(new DOMException("Aborted", "AbortError"));
			return;
		}

		const wsUrl = `${hubWsUrl()}/hindsight/timeline?${params.toString()}`;
		let socket: WebSocket | null = null;
		const measurements: Measurement[] = [];

		try {
			socket = new WebSocket(wsUrl);
			socket.binaryType = "arraybuffer";
		} catch (err) {
			reject(err);
			return;
		}

		const onAbort = () => {
			if (socket) {
				socket.close();
			}
			reject(new DOMException("Aborted", "AbortError"));
		};

		if (options?.signal) {
			options.signal.addEventListener("abort", onAbort, { once: true });
		}

		socket.onmessage = (event: MessageEvent) => {
			if (!(event.data instanceof ArrayBuffer)) {
				return;
			}

			try {
				const bytes = new Uint8Array(event.data);
				const buffer = new flatbuffers.ByteBuffer(bytes);
				const frame = MeasurementsFrame.getRootAsMeasurementsFrame(buffer);
				const count = frame.rowsLength();

				for (let i = 0; i < count; i++) {
					const row = frame.rows(i);
					if (row) {
						measurements.push(flatbufferToMeasurement(row.unpack()));
					}
				}

				if (options?.onProgress && measurements.length > 0) {
					options.onProgress(
						adaptMeasurementsToTimeline(measurements, query, symbols, excursions),
					);
				}
			} catch (err) {
				console.error("Failed to decode FlatBuffers MeasurementsFrame:", err);
			}
		};

		socket.onerror = () => {
			if (options?.signal) {
				options.signal.removeEventListener("abort", onAbort);
			}
			reject(
				new Error(
					`Hindsight request failed (WebSocket): connection error to ${wsUrl}`,
				),
			);
		};

		socket.onclose = () => {
			if (options?.signal) {
				options.signal.removeEventListener("abort", onAbort);
			}

			resolve(adaptMeasurementsToTimeline(measurements, query, symbols, excursions));
		};
	});
};

export const fetchHindsightMetricMap =
	async (): Promise<HindsightMetricMap | null> => {
		const response = await fetch(`${hubBaseUrl()}/hindsight/metric-map`);

		if (!response.ok) {
			throw new Error(
				`Hindsight request failed (${response.status}): ${await response.text()}`,
			);
		}

		return (await response.json()) as HindsightMetricMap;
	};


