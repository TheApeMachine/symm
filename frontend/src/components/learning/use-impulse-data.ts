import { useEffect, useMemo, useRef, useState } from "react";
import { atoms, DEFAULT_KERNELS, signals } from "#/collections/app";
import type { Region } from "#/collections/envelope";
import {
	createCanonicalGridNodes,
	getCanonicalPinDefs,
	getCanonicalRegionCounts,
	REGION_COUNT,
} from "./grid-pins";
import { REGION_KEYS, REGIONS } from "./regions";
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

const snm = (raw: number): number => {
	if (!Number.isFinite(raw) || raw === 0) return 0;
	return Math.abs(raw) / (1 + Math.abs(raw));
};

const regionBrightness = (region: Region): number => {
	if (
		typeof region.brightness === "number" &&
		Number.isFinite(region.brightness)
	) {
		return region.brightness;
	}

	return 0;
};

/*
symbolGridFrames returns every symbol's latest regions from the regions signal.
*/
const symbolGridFrames = (): Array<{ symbol: string; rows: Region[] }> => {
	const out: Array<{ symbol: string; rows: Region[] }> = [];
	const state = signals.regions.state ?? {};

	for (const [symbol, rows] of Object.entries(state)) {
		if (!symbol || !Array.isArray(rows)) continue;
		out.push({ symbol, rows });
	}

	out.sort((a, b) => a.symbol.localeCompare(b.symbol));
	return out;
};

const paintRegions = (
	rows: Region[],
	heldBrightness: { current: number[] },
	heldMembers: { current: number[] },
) => {
	for (let i = 0; i < REGION_COUNT; i++) {
		heldBrightness.current[i] = 0;
	}

	for (const region of rows) {
		if (region == null || typeof region.key !== "number") continue;
		if (region.key < 0 || region.key >= REGION_COUNT) continue;

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
	if (!symbol) return undefined;

	const store = signals[pin.source];
	if (!store) return undefined;

	const reading =
		store.state[symbol] ?? store.state[symbol.replace(/\//g, "-")];
	if (!reading || typeof reading !== "object") return undefined;

	const raw = (reading as Record<string, unknown>)[pin.metric];
	return typeof raw === "number" && Number.isFinite(raw) ? raw : undefined;
};

export type UseImpulseDataOptions = {
	/** When true, held readings and the map freeze until resumed. */
	paused?: boolean;
};

/*
useImpulseData paints the focus symbol's grid regions onto the map.
Full multi-symbol frames stay in signals.regions for other consumers.
*/
export const useImpulseData = (
	symbol = "",
	options: UseImpulseDataOptions = {},
): ImpulseDataState => {
	const paused = options.paused ?? false;
	const initialNodes = useMemo(() => createCanonicalGridNodes(), []);

	const heldPins = useRef<Record<string, { activation: number; snr: number }>>(
		{},
	);
	const heldBrightness = useRef<number[]>(new Array(REGION_COUNT).fill(0));
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
			gridRegions: REGION_COUNT,
			marketTick: 0,
			litSymbols: [],
		};
	});

	const lastPublishedTick = useRef(0);

	useEffect(() => {
		heldPins.current = {};
		heldBrightness.current = new Array(REGION_COUNT).fill(0);
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
				gridRegions: REGION_COUNT,
				marketTick,
				litSymbols: heldLitSymbols.current,
			});
		};

		const paintSymbol = (next: string, rows: Region[], marketTick: number) => {
			paintRegions(rows, heldBrightness, heldMembers);
			heldLitSymbols.current = [next];

			for (const pin of CANONICAL_PINS) {
				const raw = pinMetric(pin, next);
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

		const frameForFocus = (
			frames: Array<{ symbol: string; rows: Region[] }>,
		) => {
			if (!symbol) {
				return frames[0];
			}

			const dashed = symbol.replace(/\//g, "-");

			return frames.find(
				(frame) => frame.symbol === symbol || frame.symbol === dashed,
			);
		};

		const updateFromTelemetry = () => {
			if (paused) {
				return;
			}

			const tick = atoms.tick.get()?.n || lastPublishedTick.current;
			const frames = symbolGridFrames();
			const frame = frameForFocus(frames);

			if (!frame) {
				paintRegions([], heldBrightness, heldMembers);
				heldLitSymbols.current = symbol ? [symbol] : [];
				publish(tick);
				return;
			}

			paintToken.current += 1;
			paintSymbol(frame.symbol, frame.rows, tick);
		};

		publish(lastPublishedTick.current);
		updateFromTelemetry();

		const unsubs: Array<(() => void) | undefined> = [];

		const regionsSub = signals.regions.subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => regionsSub.unsubscribe());

		const tickSub = atoms.tick.subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => tickSub.unsubscribe());

		for (const kernel of DEFAULT_KERNELS) {
			const store = signals[kernel];
			if (!store) continue;
			const sub = store.subscribe(() => {
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
	}, [paused, symbol]);

	return state;
};
