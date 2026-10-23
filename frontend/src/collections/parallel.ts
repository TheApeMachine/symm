import { useSelector } from "@tanstack/react-store";
import { createStore } from "@tanstack/store";
import type { BookDeskStats } from "#/collections/longtail";
import { longtailStore } from "#/collections/longtail";

export type ParallelBookStats = BookDeskStats & { id: "A" | "B" | "C" };

export type ParallelSample = {
	at: number;
	books: Record<"A" | "B" | "C", ParallelBookStats>;
	deltaBA: number | null;
	incumbent: BookDeskStats | null;
	physics: ParallelPhysicsWire | null;
	gates: ParallelGatesWire | null;
};

export type ParallelPhysicsWire = {
	healthy?: boolean;
	kuramotoR?: number;
	kuramotoPsi?: number;
	deltaR?: number;
	guidanceSpeed?: number;
	coherenceMag2?: number;
	nullR?: number;
	spectral?: { omega?: number; linewidth?: number; magnitude?: number };
};

export type ParallelGatesWire = {
	symbols?: number;
	coherence?: number;
	familiar?: number;
	guidance?: number;
	metabolic?: number;
	all?: number;
};

export type ParallelSummaryWire = {
	tick?: number;
	books?: {
		A?: Partial<ParallelBookStats>;
		B?: Partial<ParallelBookStats>;
		C?: Partial<ParallelBookStats>;
	};
	deltaBA?: number;
	physics?: ParallelPhysicsWire;
	gates?: ParallelGatesWire;
};

const bookStats = (
	id: "A" | "B" | "C",
	raw?: Partial<ParallelBookStats>,
): ParallelBookStats => ({
	id,
	closed: raw?.closed ?? 0,
	open: raw?.open ?? 0,
	wins: raw?.wins ?? 0,
	winRate:
		typeof raw?.winRate === "number" ? raw.winRate : raw?.closed ? null : null,
	meanReturn: typeof raw?.meanReturn === "number" ? raw.meanReturn : null,
	medianReturn: null,
	worstReturn: null,
	avgHoldNs: null,
});

export const parallelStore = createStore({
	samples: [] as ParallelSample[],
	lastAt: 0,
});

const MAX = 720;

export const ingestParallelSummary = (payload: unknown, now = Date.now()) => {
	if (!payload || typeof payload !== "object") {
		return false;
	}

	const body = payload as ParallelSummaryWire;
	const incumbent = longtailStore.state.samples.at(-1)?.virtual ?? null;

	const sample: ParallelSample = {
		at: now,
		books: {
			A: bookStats("A", body.books?.A),
			B: bookStats("B", body.books?.B),
			C: bookStats("C", body.books?.C),
		},
		deltaBA: typeof body.deltaBA === "number" ? body.deltaBA : null,
		physics: body.physics ?? null,
		gates: body.gates ?? null,
		incumbent: incumbent
			? {
					closed: incumbent.closed,
					open: incumbent.open,
					wins: incumbent.wins,
					winRate: incumbent.winRate,
					meanReturn: incumbent.meanReturn,
					medianReturn: incumbent.medianReturn,
					worstReturn: incumbent.worstReturn,
					avgHoldNs: incumbent.avgHoldNs,
				}
			: null,
	};

	parallelStore.setState((state) => ({
		samples: [...state.samples, sample].slice(-MAX),
		lastAt: now,
	}));

	return true;
};

export const useParallelReport = () =>
	useSelector(parallelStore, (state) => state);
