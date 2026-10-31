import { useEffect, useMemo, useRef, useState } from "react";
import {
	DEFAULT_KERNELS,
	ensureSignalStore,
	latest,
	signals,
	workspaceFrame,
} from "#/collections/app";
import type { Value } from "#/types/value";
import {
	createCanonicalGridNodes,
	getCanonicalPinDefs,
	getCanonicalRegionCounts,
} from "./grid-pins";
import { REGION_KEYS, REGIONS } from "./position-health";
import type { ImpulseNode } from "./types";

export interface ImpulseDataState {
	data: ImpulseNode[];
	regions: Array<{
		id: number;
		strength: number;
		authority: number;
		members: number;
	}>;
	activeEvents: Array<{
		id?: number;
		label: string;
		activity: number;
		members?: number;
	}>;
	gridCells: number;
	gridRegions: number;
	/** Workspace aggregate tick index; advances even when held readings are unchanged. */
	marketTick: number;
	/** Symbol whose grid frame is currently painted on the map. */
	litSymbols: string[];
}

const CANONICAL_PINS = getCanonicalPinDefs();
const CANONICAL_COUNTS = getCanonicalRegionCounts();

type WireRegion = {
	key?: number;
	brightness?: number;
	counts?: number;
	symbol?: string;
};

const snm = (raw: number): number => {
	if (!Number.isFinite(raw) || raw === 0) return 0;
	return Math.abs(raw) / (1 + Math.abs(raw));
};

const layerArray = (frame: Value, key: string): unknown[] | undefined => {
	const direct = frame.reading(key);
	if (Array.isArray(direct)) {
		return direct;
	}

	for (const { payload } of frame.layersUnder(key)) {
		if (Array.isArray(payload)) {
			return payload;
		}
	}

	return undefined;
};

const regionBrightness = (region: WireRegion): number => {
	if (typeof region.brightness === "number" && Number.isFinite(region.brightness)) {
		return region.brightness;
	}

	return 0;
};

const frameRegions = (frame: Value | undefined): WireRegion[] => {
	if (!frame) return [];

	const rows = layerArray(frame, "regions") as WireRegion[] | undefined;
	if (!rows) return [];

	return rows.map((row) =>
		typeof row?.symbol === "string" && row.symbol !== ""
			? row
			: { ...row, symbol: frame.scope || row?.symbol },
	);
};

/*
symbolGridFrames returns every symbol's latest grid/train regions frame.
Each entry is that symbol alone — never merged across the board.
*/
const symbolGridFrames = (): Array<{ symbol: string; rows: WireRegion[] }> => {
	const out: Array<{ symbol: string; rows: WireRegion[] }> = [];
	const seen = new Set<string>();

	const ingest = (origin: "grid" | "train") => {
		const state = signals[origin]?.state;
		if (!state) return;

		for (const [scope, frame] of Object.entries(state)) {
			if (!frame || scope === "" || seen.has(scope)) continue;
			seen.add(scope);
			out.push({ symbol: scope, rows: frameRegions(frame) });
		}
	};

	ingest("grid");
	ingest("train");
	out.sort((a, b) => a.symbol.localeCompare(b.symbol));
	return out;
};

const paintRegions = (
	rows: WireRegion[],
	heldBrightness: { current: number[] },
	heldMembers: { current: number[] },
) => {
	for (let i = 0; i < 15; i++) {
		heldBrightness.current[i] = 0;
	}

	for (const region of rows) {
		if (region == null || typeof region.key !== "number") continue;
		if (region.key < 0 || region.key > 14) continue;

		heldBrightness.current[region.key] = regionBrightness(region);

		if (typeof region.counts === "number") {
			heldMembers.current[region.key] = Math.round(region.counts);
		}
	}
};

const pinMetric = (
	pin: (typeof CANONICAL_PINS)[number],
	symbol: string,
): number | undefined => {
	if (pin.scope === "aggregate") {
		const raw = latest(pin.source, "aggregate")?.metrics()[pin.metric];
		return typeof raw === "number" && Number.isFinite(raw) ? raw : undefined;
	}

	if (symbol === "") return undefined;

	const raw = latest(pin.source, symbol)?.metrics()[pin.metric];
	return typeof raw === "number" && Number.isFinite(raw) ? raw : undefined;
};

export type UseImpulseDataOptions = {
	/** When true, held readings and the map freeze until resumed. */
	paused?: boolean;
};

/*
useImpulseData paints each symbol's grid regions onto the map in turn.
No cross-symbol merge — the map shows what one symbol's grid published,
then the next, in quick succession.
*/
export const useImpulseData = (
	_symbol = "",
	options: UseImpulseDataOptions = {},
): ImpulseDataState => {
	const paused = options.paused ?? false;
	const initialNodes = useMemo(() => createCanonicalGridNodes(), []);

	const heldPins = useRef<Record<string, { activation: number; snr: number }>>(
		{},
	);
	const heldBrightness = useRef<number[]>(new Array(15).fill(0));
	const heldMembers = useRef<number[]>(
		REGION_KEYS.map((_key, idx) => Math.round(CANONICAL_COUNTS[idx] ?? 0)),
	);
	const heldLitSymbols = useRef<string[]>([]);
	const paintToken = useRef(0);

	const [state, setState] = useState<ImpulseDataState>(() => {
		const initialRegions = REGION_KEYS.map((_key, idx) => ({
			id: idx,
			strength: 0,
			authority: 0,
			members: CANONICAL_COUNTS[idx] ?? 0,
		}));

		return {
			data: initialNodes,
			regions: initialRegions,
			activeEvents: [],
			gridCells: initialNodes.length,
			gridRegions: 15,
			marketTick: 0,
			litSymbols: [],
		};
	});

	const lastPublishedTick = useRef(0);

	useEffect(() => {
		heldPins.current = {};
		heldBrightness.current = new Array(15).fill(0);
		heldMembers.current = REGION_KEYS.map((_key, idx) =>
			Math.round(CANONICAL_COUNTS[idx] ?? 0),
		);
		heldLitSymbols.current = [];
		paintToken.current += 1;

		const publish = (marketTick: number) => {
			const updatedNodes: ImpulseNode[] = CANONICAL_PINS.map((pin) => {
				const reading = heldPins.current[pin.id];
				const metricAct = reading?.activation ?? 0;
				const regionLit = heldBrightness.current[pin.cluster] ?? 0;
				const activation = Math.max(metricAct, regionLit);

				return {
					id: pin.id,
					label: `${pin.source}:${pin.metric}`,
					cluster: pin.cluster,
					snr: reading?.snr ?? (activation >= 0.5 ? 2.5 : 1),
					activation,
				};
			});

			const regionsList = REGION_KEYS.map((_key, idx) => {
				const strength = heldBrightness.current[idx] ?? 0;

				return {
					id: idx,
					strength,
					authority: strength >= 0.5 ? 1 : strength,
					members: heldMembers.current[idx] ?? 0,
				};
			});

			const activeEventsList = REGION_KEYS.map((regionKey, idx) => {
				const meta = REGIONS[regionKey];
				const strength = heldBrightness.current[idx] ?? 0;

				return {
					id: idx,
					label: meta ? `${meta.id} ${meta.title}` : regionKey,
					activity: strength,
					members: heldMembers.current[idx],
				};
			});

			lastPublishedTick.current = marketTick;

			setState({
				data: updatedNodes,
				regions: regionsList,
				activeEvents: activeEventsList,
				gridCells: updatedNodes.length,
				gridRegions: 15,
				marketTick,
				litSymbols: heldLitSymbols.current,
			});
		};

		const paintSymbol = (
			symbol: string,
			rows: WireRegion[],
			marketTick: number,
		) => {
			paintRegions(rows, heldBrightness, heldMembers);
			heldLitSymbols.current = [symbol];

			for (const pin of CANONICAL_PINS) {
				const raw = pinMetric(pin, symbol);
				if (raw === undefined) {
					heldPins.current[pin.id] = { activation: 0, snr: 1 };
					continue;
				}

				const act = snm(raw);
				heldPins.current[pin.id] = {
					activation: act,
					snr: act >= 0.5 ? 2.5 : 1.0,
				};
			}

			publish(marketTick);
		};

		const updateFromTelemetry = () => {
			if (paused) {
				return;
			}

			const aggregate = workspaceFrame();
			const marketTick =
				aggregate && aggregate.tick !== undefined
					? Number(aggregate.tick)
					: lastPublishedTick.current;
			const tick = Number.isFinite(marketTick)
				? marketTick
				: lastPublishedTick.current;

			const frames = symbolGridFrames();
			if (frames.length === 0) {
				paintRegions([], heldBrightness, heldMembers);
				heldLitSymbols.current = [];
				publish(tick);
				return;
			}

			const token = ++paintToken.current;
			let index = 0;

			const step = () => {
				if (token !== paintToken.current) return;
				if (index >= frames.length) return;

				const frame = frames[index];
				index += 1;
				paintSymbol(frame.symbol, frame.rows, tick);

				if (index < frames.length) {
					requestAnimationFrame(step);
				}
			};

			step();
		};

		publish(lastPublishedTick.current);
		updateFromTelemetry();

		const unsubs: Array<(() => void) | undefined> = [];

		const trainSub = ensureSignalStore("train").subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => trainSub.unsubscribe());

		const workspaceSub = signals.workspace.subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => workspaceSub.unsubscribe());

		const gridSub = ensureSignalStore("grid").subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => gridSub.unsubscribe());

		for (const kernel of DEFAULT_KERNELS) {
			const sub = ensureSignalStore(kernel).subscribe(() => {
				updateFromTelemetry();
			});
			unsubs.push(() => sub.unsubscribe());
		}

		return () => {
			paintToken.current += 1;
			for (const u of unsubs) {
				u?.();
			}
		};
	}, [paused]);

	return state;
};
