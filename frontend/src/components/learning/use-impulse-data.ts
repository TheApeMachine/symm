import { useEffect, useMemo, useState } from "react";
import { useSelector } from "@tanstack/react-store";
import {
	atoms,
	clockAtom,
	signalFor,
	signals,
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
}

const CANONICAL_PINS = getCanonicalPinDefs();
const CANONICAL_COUNTS = getCanonicalRegionCounts();

/*
useImpulseData tracks the live SOM grid cells and confluence regions from signals.training
and peer signal stores for the given symbol.
*/
export const useImpulseData = (symbol: string): ImpulseDataState => {
	const initialNodes = useMemo(() => createCanonicalGridNodes(), []);

	/*
	The signal layers ride the raw market tick through the websocket ingest:
	no store of their own fires when they arrive. The tick atom advances on
	every inbound batch, so re-deriving on it re-reads the signal rings each
	tick instead of only when training happens to publish.
	*/
	const tickCount = useSelector(atoms.tickCount);

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
			}> = [];
			if (latest) {
				for (const { payload } of latest.layersUnder("grid/regions")) {
					if (!Array.isArray(payload)) continue;
					for (const region of payload) {
						if (region && typeof region.key === "number") {
							gridRegions.push(region);
						}
					}
				}
			}
			if (latest) {
				for (const { suffix, payload } of latest.layersUnder(
					"train/regions",
				)) {
					const first = Array.isArray(payload)
						? payload[0]
						: payload;
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
						trainingMetrics[`region_brightness:${region}`] =
							first;
					}
				}
			}

			// Read specific metric activations off the aggregate tick.
			const kernelMetricSnm: Record<
				string,
				{ activation: number; snr: number }
			> = {};
			for (const pin of CANONICAL_PINS) {
				const payloads = signalFor(pin.source, symbol);
				const payload = payloads[pin.metric];
				if (!Array.isArray(payload) || payload.length === 0) continue;

				const raw = Number(payload[0]);
				if (!Number.isFinite(raw)) continue;

				const act = Math.min(1, Math.max(0, Math.abs(raw)));
				const snr = act >= 0.5 ? 2.5 : 1.0;
				kernelMetricSnm[pin.id] = { activation: act, snr };
			}

			// Read or derive brightness and member counts for each of the 15 regions (R00..R14)
			const regionBrightness: number[] = new Array(15).fill(0);
			const regionMembers: number[] = new Array(15).fill(0);

			for (let i = 0; i < 15; i++) {
				const key = `R${String(i).padStart(2, "0")}`;
				const explicitBrightness = trainingMetrics[`region_brightness:${key}`];
				const explicitMembers = trainingMetrics[`region_members:${key}`];
				const gridRegion = gridRegions.find((region) => region.key === i);

				if (gridRegion) {
					regionBrightness[i] = gridRegion.brightness;
					regionMembers[i] = gridRegion.counts;
					continue;
				}

				if (explicitBrightness !== undefined && explicitBrightness > 0) {
					regionBrightness[i] = explicitBrightness;
					regionMembers[i] = Math.round(
						explicitMembers ?? CANONICAL_COUNTS[i] ?? 0,
					);
				} else {
					// Derive region brightness as the average activation of its member pins (matching nomagique/store/grid.go)
					let pinSum = 0;
					let pinCount = 0;
					for (const pin of CANONICAL_PINS) {
						if (pin.cluster === i) {
							const reading = kernelMetricSnm[pin.id];
							if (reading) {
								pinSum += reading.activation;
								pinCount++;
							}
						}
					}
					regionBrightness[i] = pinCount > 0 ? pinSum / pinCount : 0;
					regionMembers[i] = Math.round(
						explicitMembers ?? CANONICAL_COUNTS[i] ?? 0,
					);
				}
			}

			// Update all 81 canonical nodes
			const updatedNodes: ImpulseNode[] = CANONICAL_PINS.map((pin) => {
				const directReading = kernelMetricSnm[pin.id];
				const regBrightness = regionBrightness[pin.cluster] ?? 0;
				const activation =
					directReading !== undefined && directReading.activation > 0
						? directReading.activation
						: regBrightness;
				const snr = directReading?.snr ?? (activation >= 0.5 ? 2.5 : 1.0);

				return {
					id: pin.id,
					label: `${pin.source}:${pin.metric}`,
					cluster: pin.cluster,
					snr,
					activation,
				};
			});

			// Build 15 regions
			const regionsList = REGION_KEYS.map((_key, idx) => ({
				id: idx,
				strength: regionBrightness[idx],
				authority: regionBrightness[idx] >= 0.5 ? 1 : regionBrightness[idx],
				members: regionMembers[idx],
			}));

			// Build active events sorted brightest first
			const activeEventsList: Array<{
				id?: number;
				label: string;
				activity: number;
				members?: number;
			}> = [];

			for (let i = 0; i < 15; i++) {
				const strength = regionBrightness[i];
				if (strength > 0.02) {
					const meta = REGIONS[REGION_KEYS[i]];
					activeEventsList.push({
						id: i,
						label: meta
							? `${meta.id} ${meta.title}`
							: `R${String(i).padStart(2, "0")}`,
						activity: strength,
						members: regionMembers[i],
					});
				}
			}

			activeEventsList.sort((a, b) => b.activity - a.activity);

			setState({
				data: updatedNodes,
				regions: regionsList,
				activeEvents: activeEventsList,
				gridCells: updatedNodes.length,
				gridRegions: 15,
			});
		};

		// Run immediately
		updateFromTelemetry();

		const unsubs: Array<(() => void) | undefined> = [];

		// Clock atom fires on every inbound websocket batch
		const clockSub = clockAtom.subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => clockSub.unsubscribe());

		// Training store subscription
		const trainingSub = signals.training?.subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => trainingSub?.unsubscribe?.());

		// The aggregate tick carries every kernel's layers.
		const workspaceSub = signals.workspace.subscribe(() => {
			updateFromTelemetry();
		});
		unsubs.push(() => workspaceSub.unsubscribe());

		return () => {
			for (const u of unsubs) {
				u?.();
			}
		};
	}, [symbol, tickCount]);

	return state;
};
