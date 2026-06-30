import { useEffect } from "react";
import { appStore } from "#/collections/app";
import type { ArtifactFrame } from "#/collections/artifacts";
import { balancesStore } from "#/collections/balances";
import { cognitiveStore } from "#/collections/cognitive";
import { decisionsStore } from "#/collections/decisions";
import { executionsStore } from "#/collections/executions";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { playbookStore } from "#/collections/playbook";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import { tickStore } from "#/collections/tick";
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const WIRE_ERROR_LOG_INTERVAL_MS = 5000;
const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;
const UI_FLUSH_INTERVAL_MS = 1000 / 60;

let lastWireErrorAt = 0;

const updateTick = (frame: ArtifactFrame) => {
	tickStore.actions.updateFrame(frame);
	const count = frame.count;
	const phase = frame.phase;
	const candidates = frame.candidates;

	if (typeof count === "number") {
		appStore.actions.updateStoryTicks(count);
	}
	if (typeof phase === "string") {
		appStore.actions.updateEnginePhase(phase);
	}
	if (typeof candidates === "number") {
		appStore.actions.updatePlaybookEvaluations(candidates);
	}
};

type FrameRoute = {
	latest?: (frame: ArtifactFrame) => void;
	batch?: (frames: ArtifactFrame[]) => void;
};

const latest = (frames: ArtifactFrame[]): ArtifactFrame | null =>
	frames[frames.length - 1] ?? null;

const updateRegimeBatch = (frames: ArtifactFrame[]) => {
	measurementsStore.actions.updateReadings(frames);
	const frame = latest(frames);

	if (frame !== null) {
		appStore.actions.stashRegimeFrame(frame);
	}
};

const updateManifoldBatch = (frames: ArtifactFrame[]) => {
	manifoldStore.actions.updateFrames(frames);
	const frame = latest(frames);

	if (frame !== null) {
		appStore.actions.stashManifoldFrame(frame);
	}
};

const routes: Record<string, FrameRoute> = {
	tick: { latest: updateTick },
	measurement: { batch: measurementsStore.actions.updateReadings },
	resonance: { batch: resonanceStore.actions.updateFrames },
	balances: { batch: balancesStore.actions.updateFrames },
	executions: { batch: executionsStore.actions.updateFrames },
	fill: { batch: executionsStore.actions.updateFrames },
	quote: { batch: executionsStore.actions.updateFrames },
	orders: { batch: ordersStore.actions.updateFrames },
	order: { batch: ordersStore.actions.updateFrames },
	stoploss: { batch: ordersStore.actions.updateFrames },
	regime: { batch: updateRegimeBatch },
	manifold: { batch: updateManifoldBatch },
	buy: { batch: decisionsStore.actions.updateFrames },
	sell: { batch: decisionsStore.actions.updateFrames },
	decision: { batch: decisionsStore.actions.updateFrames },
	decisions: { batch: decisionsStore.actions.updateFrames },
	positions: { batch: positionsStore.actions.updateFrames },
	walk: { latest: playbookStore.actions.updateFrame },
	cognitive: { latest: cognitiveStore.actions.updateFrame },
};

export const flushBufferedFrames = (
	frames: ArtifactFrame[],
	routeTable: Record<string, FrameRoute> = routes,
) => {
	const byRole = new Map<string, ArtifactFrame[]>();

	for (const frame of frames) {
		if (typeof frame.role !== "string" || frame.role === "") {
			continue;
		}

		const roleFrames = byRole.get(frame.role);
		if (roleFrames === undefined) {
			byRole.set(frame.role, [frame]);
			continue;
		}

		roleFrames.push(frame);
	}

	for (const [role, roleFrames] of byRole) {
		const route = routeTable[role];

		if (route === undefined) {
			continue;
		}

		if (route.batch !== undefined) {
			route.batch(roleFrames);
			continue;
		}

		if (route.latest !== undefined) {
			const frame = latest(roleFrames);

			if (frame !== null) {
				route.latest(frame);
			}
		}
	}
};

const wireBufferFromMessage = async (
	data: MessageEvent["data"],
): Promise<ArrayBuffer | null> => {
	if (data instanceof ArrayBuffer) {
		return data;
	}

	if (data instanceof Blob) {
		return data.arrayBuffer();
	}

	return null;
};

const logWireError = (error: unknown) => {
	const now = Date.now();

	if (now - lastWireErrorAt < WIRE_ERROR_LOG_INTERVAL_MS) {
		return;
	}

	lastWireErrorAt = now;
	console.error("websocket frame parse failed", error);
};

export const WsFeed = () => {
	const { updateOnline } = appStore.actions;

	useEffect(() => {
		let closedByUnmount = false;
		let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
		let flushTimer: ReturnType<typeof setTimeout> | null = null;
		let attempt = 0;
		let socket: WebSocket | null = null;
		let decodeWorker: Worker | null = null;
		const pendingFrames: ArtifactFrame[] = [];

		const enqueueFrame = (frame: ArtifactFrame | null) => {
			if (frame === null) {
				return;
			}

			pendingFrames.push(frame);

			if (flushTimer !== null) {
				return;
			}

			flushTimer = setTimeout(() => {
				flushTimer = null;
				const frames = pendingFrames.splice(0);

				if (frames.length > 0) {
					flushBufferedFrames(frames);
				}
			}, UI_FLUSH_INTERVAL_MS);
		};

		const ensureDecodeWorker = (): Worker | null => {
			if (decodeWorker !== null) {
				return decodeWorker;
			}

			try {
				decodeWorker = new Worker(
					new URL("./websocket-decode.worker.ts", import.meta.url),
					{ type: "module" },
				);
				decodeWorker.addEventListener(
					"message",
					(
						event: MessageEvent<{
							frame: ArtifactFrame | null;
							error?: string;
						}>,
					) => {
						if (event.data.error !== undefined) {
							logWireError(event.data.error);
							return;
						}

						enqueueFrame(event.data.frame);
					},
				);
				decodeWorker.addEventListener("error", (event) => {
					logWireError(event.message);
				});
			} catch (error) {
				logWireError(error);
				decodeWorker = null;
			}

			return decodeWorker;
		};

		const decodeAndQueue = async (buffer: ArrayBuffer) => {
			const worker = ensureDecodeWorker();

			if (worker !== null) {
				worker.postMessage({ buffer }, [buffer]);
				return;
			}

			enqueueFrame(await decodePackedArtifactWire(buffer));
		};

		const scheduleReconnect = () => {
			if (closedByUnmount || reconnectTimer !== null) {
				return;
			}

			const delay = Math.min(
				RECONNECT_MAX_MS,
				RECONNECT_BASE_MS * 2 ** attempt,
			);
			attempt += 1;
			reconnectTimer = setTimeout(() => {
				reconnectTimer = null;
				connect();
			}, delay);
		};

		const connect = () => {
			socket = new WebSocket(socketUrl);
			socket.binaryType = "arraybuffer";

			socket.addEventListener("open", () => {
				attempt = 0;
				updateOnline(true);
			});

			socket.addEventListener("close", () => {
				updateOnline(false);
				scheduleReconnect();
			});

			socket.addEventListener("error", () => {
				socket?.close();
			});

			socket.addEventListener("message", (event) => {
				void (async () => {
					try {
						const buffer = await wireBufferFromMessage(event.data);

						if (buffer === null) {
							return;
						}

						await decodeAndQueue(buffer);
					} catch (error) {
						logWireError(error);
					}
				})();
			});
		};

		connect();

		return () => {
			closedByUnmount = true;
			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
			}
			if (flushTimer !== null) {
				clearTimeout(flushTimer);
			}
			decodeWorker?.terminate();
			socket?.close();
		};
	}, [updateOnline]);

	return null;
};
