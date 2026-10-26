import { useEffect, useMemo, useRef, useState } from "react";
import { signalFor, signals, workspaceFrame } from "#/collections/app";
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
}

const CANONICAL_PINS = getCanonicalPinDefs();
const CANONICAL_COUNTS = getCanonicalRegionCounts();

/*
useImpulseData tracks the live SOM grid cells and confluence regions from the
workspace aggregate tick (and training step frames when present) for the symbol.
*/
export const useImpulseData = (symbol: string): ImpulseDataState => {
	const initialNodes = useMemo(() => createCanonicalGridNodes(), []);

	/*
	A tick carries whichever kernels and regions fired. The rest are absent,
	not dark. Held readings stay until a later tick names that pin or region.
	*/
	const heldPins = useRef<Record<string, { activation: number; snr: number }>>(
		{},
	);
	const heldBrightness = useRef<number[]>(new Array(15).fill(0));
	const heldMembers = useRef<number[]>(
		REGION_KEYS.map((_key, idx) => Math.round(CANONICAL_COUNTS[idx] ?? 0)),
	);

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
		};
	});

	useEffect(() => {
		if (!symbol) return;

		heldPins.current = {};
		heldBrightness.current = new Array(15).fill(0);
		heldMembers.current = REGION_KEYS.map((_key, idx) =>
			Math.round(CANONICAL_COUNTS[idx] ?? 0),
		);

		const publish = () => {
			const updatedNodes: ImpulseNode[] = CANONICAL_PINS.map((pin) => {
				const reading = heldPins.current[pin.id];
				const regionLit = heldBrightness.current[pin.cluster] ?? 0;
				const metricAct = reading?.activation ?? 0;
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

			setState({
				data: updatedNodes,
				regions: regionsList,
				activeEvents: activeEventsList,
				gridCells: updatedNodes.length,
				gridRegions: 15,
			});
		};

		const updateFromTelemetry = () => {
			const trainingState = signals.training?.state;

			// Look for the training frame for the symbol, or any scope.
			let latest: Value | null = null;
			if (trainingState) {
				if (symbol && trainingState[symbol]) {
					latest = trainingState[symbol];
				} else if (trainingState.learner) {
					latest = trainingState.learner;
				} else {
					for (const frame of Object.values(trainingState)) {
						if (frame) {
							latest = frame;
							break;
						}
					}
				}
			}

			// Build fast lookup for training metrics from the layer payloads
			const trainingMetrics: Record<string, number> = {};
			/*
			The grid's own frame: grid/regions/<symbol> carries the regions
			the grid accepted this tick — key, deformation, member counts,
			and the brightness after its midpoint cut. The map renders
			these directly.
			*/
			const gridRegions: Array<{
				key: number;
				deformation: number;
				brightness: number;
				counts: number;
				symbol?: string;
			}> = [];
			let sawRegionLayer = false;
			/*
			Training.Step appends the lit regions onto the workspace tick
			as train/regions. A training-origin frame may also carry
			grid/regions. Either shape is the region list the map draws.
			*/
			const regionFrames = [workspaceFrame(), latest];
			for (const frame of regionFrames) {
				if (!frame) continue;
				for (const key of ["train/regions", "grid/regions"]) {
					for (const { payload } of frame.layersUnder(key)) {
						if (!Array.isArray(payload)) continue;
						sawRegionLayer = true;
						for (const region of payload) {
							if (!region || typeof region.key !== "number") continue;
							if (
								typeof region.symbol === "string" &&
								region.symbol !== "" &&
								region.symbol !== symbol
							) {
								continue;
							}
							gridRegions.push(region);
						}
					}
				}
				if (symbol) {
					for (const { payload } of frame.layersUnder(
						`stage/${symbol}/regions`,
					)) {
						if (!Array.isArray(payload)) continue;
						sawRegionLayer = true;
						for (const region of payload) {
							if (!region || typeof region.key !== "number") continue;
							gridRegions.push(region);
						}
					}
				}
			}
			if (latest) {
				for (const { suffix, payload } of latest.layersUnder("train/regions")) {
					const first = Array.isArray(payload) ? payload[0] : payload;
					if (typeof first !== "number") {
						continue;
					}

					/*
					train/regions/<symbol>/<R##> carries brightness per
					accepted region; train/regions/<symbol>/<token path>
					carries the whole sentence. The region is the R##
					segment wherever it appears in the suffix.
					*/
					const region = suffix.match(/R\d{2}/)?.[0];

					if (region) {
						trainingMetrics[`region_brightness:${region}`] = first;
					}
				}
			}

			let changed = false;

			/*
			A kernel layer names its metrics. Pins whose metric is on this
			tick take that reading, including zero. Pins the tick does not
			name keep the reading they already hold.
			*/
			for (const pin of CANONICAL_PINS) {
				const payloads = signalFor(pin.source, symbol);
				const payload = payloads[pin.metric];
				if (!Array.isArray(payload) || payload.length === 0) continue;

				const raw = Number(payload[0]);
				if (!Number.isFinite(raw)) continue;

				const act = Math.min(1, Math.max(0, Math.abs(raw)));
				const snr = act >= 0.5 ? 2.5 : 1.0;
				const prev = heldPins.current[pin.id];
				if (prev && prev.activation === act && prev.snr === snr) {
					continue;
				}

				heldPins.current[pin.id] = { activation: act, snr };
				changed = true;
			}

			/*
			Each tick's train/regions row is the grid's lit set for this
			symbol (post midpoint cut). Regions absent from that row read
			as zero so consistently dead slots stay visible in the panel.
			*/
			if (sawRegionLayer) {
				for (let i = 0; i < 15; i++) {
					const regionKey = `R${String(i).padStart(2, "0")}`;
					const explicitBrightness =
						trainingMetrics[`region_brightness:${regionKey}`];
					const explicitMembers =
						trainingMetrics[`region_members:${regionKey}`];
					const gridRegion = gridRegions.find((region) => region.key === i);

					let brightness = 0;
					let members: number | undefined;

					if (gridRegion && typeof gridRegion.brightness === "number") {
						brightness = gridRegion.brightness;
						if (typeof gridRegion.counts === "number") {
							members = gridRegion.counts;
						}
					} else if (explicitBrightness !== undefined) {
						brightness = explicitBrightness;
						if (explicitMembers !== undefined) {
							members = Math.round(explicitMembers);
						}
					}

					if (heldBrightness.current[i] !== brightness) {
						heldBrightness.current[i] = brightness;
						changed = true;
					}

					if (members !== undefined && heldMembers.current[i] !== members) {
						heldMembers.current[i] = members;
						changed = true;
					}
				}
			}

			if (changed) publish();
		};

		// A symbol change drops the previous symbol's readings first.
		publish();
		updateFromTelemetry();

		const unsubs: Array<(() => void) | undefined> = [];

		// Training frames are not on the workspace tick.
		const trainingSub = signals.training?.subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => trainingSub?.unsubscribe?.());

		/*
		The aggregate tick carries every kernel's layers, and the market
		clock advances with that same frame, so one subscription covers both.
		*/
		const workspaceSub = signals.workspace.subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => workspaceSub.unsubscribe());

		return () => {
			for (const u of unsubs) {
				u?.();
			}
		};
	}, [symbol]);

	return state;
};
