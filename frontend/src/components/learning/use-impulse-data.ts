import { useEffect, useMemo, useState } from "react";
import {
	clockAtom,
	DEFAULT_KERNELS,
	type RingBuffer,
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

			// Look for training ring for symbol, learner, or any non-empty ring
			let trainingRing: RingBuffer<Value> | null = null;
			if (trainingState) {
				if (
					symbol &&
					trainingState[symbol] &&
					!trainingState[symbol].isEmpty()
				) {
					trainingRing = trainingState[symbol];
				} else if (trainingState.learner && !trainingState.learner.isEmpty()) {
					trainingRing = trainingState.learner;
				} else {
					for (const r of Object.values(trainingState)) {
						if (r && !r.isEmpty()) {
							trainingRing = r;
							break;
						}
					}
				}
			}

			// Build fast lookup for training metrics from the layer payloads
			const trainingMetrics: Record<string, number> = {};
			if (
				trainingRing &&
				typeof trainingRing.getBufferLength === "function" &&
				trainingRing.getBufferLength() > 0
			) {
				const latest = trainingRing.get(trainingRing.getBufferLength() - 1);
				if (latest) {
					for (const { suffix, payload } of latest.layersUnder("train/regions")) {
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
			}

			// Read specific metric activations directly from the 10 signal kernel stores
			const kernelMetricSnm: Record<
				string,
				{ activation: number; snr: number }
			> = {};
			for (const pin of CANONICAL_PINS) {
				const store = signals[pin.source];
				if (!store?.state) continue;
				const kernelRing = store.state[symbol] ?? store.state["BTC/USD"];
				if (!kernelRing || typeof kernelRing.getLast !== "function") continue;
				const row = kernelRing.getLast();
				if (!row) continue;

				for (const { payload } of row.layersUnder(pin.metric)) {
					if (!Array.isArray(payload) || payload.length === 0) continue;

					const raw = Number(payload[0]);
					if (!Number.isFinite(raw)) continue;

					const act = Math.min(1, Math.max(0, Math.abs(raw)));
					const snr = act >= 0.5 ? 2.5 : 1.0;
					kernelMetricSnm[pin.id] = { activation: act, snr };
					break;
				}
			}

			// Read or derive brightness and member counts for each of the 15 regions (R00..R14)
			const regionBrightness: number[] = new Array(15).fill(0);
			const regionMembers: number[] = new Array(15).fill(0);

			for (let i = 0; i < 15; i++) {
				const key = `R${String(i).padStart(2, "0")}`;
				const explicitBrightness = trainingMetrics[`region_brightness:${key}`];
				const explicitMembers = trainingMetrics[`region_members:${key}`];

				if (explicitBrightness !== undefined && explicitBrightness > 0) {
					regionBrightness[i] = explicitBrightness;
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
				}

				regionMembers[i] = Math.round(
					explicitMembers ?? CANONICAL_COUNTS[i] ?? 0,
				);
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

		// Peer kernel signal subscriptions
		for (const kernel of DEFAULT_KERNELS) {
			const store = signals[kernel];
			if (store) {
				const sub = store.subscribe(() => {
					updateFromTelemetry();
				});
				unsubs.push(() => sub?.unsubscribe?.());
			}
		}

		return () => {
			for (const u of unsubs) {
				u?.();
			}
		};
	}, [symbol]);

	return state;
};
