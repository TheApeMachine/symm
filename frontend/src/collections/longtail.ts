import { createStore } from "@tanstack/store";
import { useSelector } from "@tanstack/react-store";
import { positionRows, signals } from "#/collections/app";
import type { Book } from "#/collections/book";
import { sequiturStore } from "#/collections/sequitur";
import { closedOnBook, onBook, openOnBook, positionReadout } from "#/components/book-readout";
import { ingestDeskClosedLots } from "#/lib/lifecycle";

export type Trend = "up" | "down" | "flat";

export type BookDeskStats = {
	closed: number;
	open: number;
	winRate: number | null;
	meanReturn: number | null;
};

export type LongtailSample = {
	at: number;
	paper: BookDeskStats;
	virtual: BookDeskStats;
	gatePassRate: number | null;
	policyStoredRate: number | null;
	blockedRate: number | null;
};

export type LongtailInsight = {
	id: string;
	title: string;
	body: string;
	trend: Trend;
};

const MAX_SAMPLES = 180;
const SAMPLE_EVERY_MS = 5_000;

const summarizeBook = (
	rows: any[],
	book: Book,
): BookDeskStats => {
	const onThisBook = rows.filter((row) => onBook(row, book));
	const closed = closedOnBook(rows, book);
	const returns = closed
		.map((row) => positionReadout(row).returnPct)
		.filter((value): value is number => value !== null);

	return {
		closed: closed.length,
		open: openOnBook(onThisBook, book).length,
		winRate:
			returns.length > 0
				? returns.filter((value) => value > 0).length / returns.length
				: null,
		meanReturn:
			returns.length > 0
				? returns.reduce((sum, value) => sum + value, 0) / returns.length
				: null,
	};
};

const summarizeSequitur = () => {
	const feeds = Object.values(sequiturStore.state);
	if (feeds.length === 0) {
		return {
			gatePassRate: null,
			policyStoredRate: null,
			blockedRate: null,
		};
	}

	let gateTotal = 0;
	let gatePass = 0;
	let policyTotal = 0;
	let policyStored = 0;
	let blocked = 0;

	for (const feed of feeds) {
		const frame = feed.latest;
		if (!frame) {
			continue;
		}

		if (frame.gate) {
			gateTotal += 1;
			if (frame.gate.passed) {
				gatePass += 1;
			}
		}

		policyTotal += 1;
		if (frame.policy.stored) {
			policyStored += 1;
		}

		if (frame.blocked) {
			blocked += 1;
		}
	}

	return {
		gatePassRate: gateTotal > 0 ? gatePass / gateTotal : null,
		policyStoredRate: policyTotal > 0 ? policyStored / policyTotal : null,
		blockedRate: policyTotal > 0 ? blocked / policyTotal : null,
	};
};

const trendOf = (current: number | null, prior: number | null): Trend => {
	if (
		current === null ||
		prior === null ||
		!Number.isFinite(current) ||
		!Number.isFinite(prior)
	) {
		return "flat";
	}

	if (Math.abs(current - prior) < 0.01) {
		return "flat";
	}

	return current > prior ? "up" : "down";
};

export const buildInsights = (
	samples: LongtailSample[],
	book: Book,
): LongtailInsight[] => {
	if (samples.length < 2) {
		return [
			{
				id: "warmup",
				title: `Collecting ${book} baseline`,
				body: `Samples arrive from ${book} lots and live grammar frames. The trade metrics use only the selected book; gate and policy metrics remain shared.`,
				trend: "flat",
			},
		];
	}

	const latest = samples[samples.length - 1];
	const previous = samples[Math.max(0, samples.length - 12)];
	const latestBook = latest[book];
	const previousBook = previous[book];
	const insights: LongtailInsight[] = [];

	const winTrend = trendOf(latestBook.winRate, previousBook.winRate);

	insights.push({
		id: "win-rate",
		title: `Round-trip edge (${book})`,
		body:
			latestBook.winRate === null
				? `No priced ${book} round trips yet — edge stays unknown until a lot has execution costs, fees, and a close.`
				: winTrend === "up"
					? `${book} win rate is climbing versus the earlier sample.`
					: winTrend === "down"
						? `${book} win rate is slipping.`
						: `${book} win rate is stable.`,
		trend: winTrend,
	});

	const gateTrend = trendOf(latest.gatePassRate, previous.gatePassRate);
	insights.push({
		id: "gate",
		title: "Surprise gate",
		body:
			latest.gatePassRate === null
				? "Grammar frames have not reported a gate yet."
				: gateTrend === "up"
					? "More proposed chords are passing the surprise gate — the symbol coders look settled and less noisy."
					: gateTrend === "down"
						? "Fewer chords pass the gate. Tokens may be firing into cold or still-warming coders; accuracy drops when the gate blocks often."
						: "Gate pass rate is flat — neither warming nor degrading on this horizon.",
		trend: gateTrend,
	});

	const policyTrend = trendOf(
		latest.policyStoredRate,
		previous.policyStoredRate,
	);
	insights.push({
		id: "policy",
		title: "Policy storage",
		body:
			latest.policyStoredRate === null
				? "No policy snapshots on the wire yet."
				: policyTrend === "up"
					? "More live paths are matching stored enter/exit policies — learning output is being reused, not only explored."
					: policyTrend === "down"
						? "Stored policy matches are thinning. Either paths are novel or train has not published fresh keys for current regions."
						: "Policy match rate is steady.",
		trend: policyTrend,
	});

	return insights;
};

export const longtailStore = createStore({
	samples: [] as LongtailSample[],
	lastSampleAt: 0,
});

const pushSample = (sample: LongtailSample) => {
	longtailStore.setState((state) => {
		const samples = [...state.samples, sample].slice(-MAX_SAMPLES);
		return {
			samples,
			lastSampleAt: sample.at,
		};
	});
};

const readPositions = () => positionRows(signals.positions, "all");

let recorderStarted = false;

export const ensureLongtailRecorder = () => {
	if (recorderStarted) {
		return;
	}

	recorderStarted = true;

	const tick = () => {
		const desk = readPositions();
		ingestDeskClosedLots(desk);
		const seqStats = summarizeSequitur();

		pushSample({
			at: Date.now(),
			paper: summarizeBook(desk, "paper"),
			virtual: summarizeBook(desk, "virtual"),
			gatePassRate: seqStats.gatePassRate,
			policyStoredRate: seqStats.policyStoredRate,
			blockedRate: seqStats.blockedRate,
		});
	};

	tick();
	window.setInterval(tick, SAMPLE_EVERY_MS);
};

export const useLongtailReport = () =>
	useSelector(longtailStore, (state) => state);