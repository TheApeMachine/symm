import { createStore } from "@tanstack/store";
import { atoms, direct } from "#/collections/app";
import type { PipelineFrame, PipelineNode } from "#/collections/pipeline";

/*
BURST_MS is how long a live pulse keeps an outbound edge visually lit.
*/
export const BURST_MS = 180;

export type DiagBurstState = Record<string, number>;

export const diagBursts = createStore<DiagBurstState>({});

type DiagPulse = {
	id?: string;
	at?: number;
	n?: number;
	last_ns?: number;
	ema_ns?: number;
	queue?: number;
};

const patchNodes = (
	nodes: PipelineNode[],
	pulses: DiagPulse[],
): { nodes: PipelineNode[]; changed: boolean } => {
	const byPulse = new Map<string, DiagPulse>();
	for (const pulse of pulses) {
		if (typeof pulse.id === "string" && pulse.id) {
			byPulse.set(pulse.id, pulse);
		}
	}

	if (byPulse.size === 0) {
		return { nodes, changed: false };
	}

	let changed = false;
	const next = nodes.map((node) => {
		const pulse = byPulse.get(node.id);
		if (!pulse) return node;

		changed = true;
		return {
			...node,
			n: typeof pulse.n === "number" ? pulse.n : node.n,
			last_ns:
				typeof pulse.last_ns === "number" ? pulse.last_ns : node.last_ns,
			ema_ns: typeof pulse.ema_ns === "number" ? pulse.ema_ns : node.ema_ns,
			queue: typeof pulse.queue === "number" ? pulse.queue : node.queue,
			seen_at: typeof pulse.at === "number" ? pulse.at : node.seen_at,
		};
	});

	return { nodes: next, changed };
};

const rememberBursts = (pulses: DiagPulse[]) => {
	const at = Date.now();
	diagBursts.setState((prev) => {
		const next = { ...prev };
		for (const pulse of pulses) {
			if (typeof pulse.id === "string" && pulse.id) {
				next[pulse.id] = at;
			}
		}
		return next;
	});
};

/*
applyDiagPulse handles a single live measurement (legacy) or one row.
*/
export const applyDiagPulse = (raw: unknown) => {
	if (!raw || typeof raw !== "object" || Array.isArray(raw)) return;
	applyDiagPulses([raw as DiagPulse]);
};

/*
applyDiagPulses applies one coalesced hub batch — one React update for many stages.
*/
export const applyDiagPulses = (raw: unknown) => {
	if (!Array.isArray(raw) || raw.length === 0) return;

	const pulses = raw.filter(
		(row): row is DiagPulse =>
			!!row && typeof row === "object" && !Array.isArray(row),
	);
	if (pulses.length === 0) return;

	rememberBursts(pulses);

	const frame = atoms.pipeline.get() as PipelineFrame;
	if (!frame?.nodes?.length) return;

	const { nodes, changed } = patchNodes(frame.nodes, pulses);
	if (changed) {
		atoms.pipeline.set({ ...frame, nodes });
	}
};

export const isBursting = (
	bursts: DiagBurstState,
	id: string | undefined,
	now = Date.now(),
): boolean => {
	if (!id) return false;
	const at = bursts[id];
	return typeof at === "number" && now - at < BURST_MS;
};

direct.diag_pulses = (value) => {
	applyDiagPulses(value);
};

direct.diag_pulse = (value) => {
	applyDiagPulse(value);
};
