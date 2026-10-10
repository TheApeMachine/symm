import {
	decodeManifoldMeasurement,
	type FluidFields,
	type FluidParticleFrame,
	type FluidPhase,
} from "./wire";
import { errorAtom, focusAtom, onlineAtom, signals } from "#/collections/app";

export type FluidFeedState = "connecting" | "connected" | "disconnected";

export type FluidFeedHandlers = {
	onFields: (fields: FluidFields) => void;
	onParticles: (particles: FluidParticleFrame) => void;
	onPhase: (phase: FluidPhase) => void;
	onState: (state: FluidFeedState) => void;
	onError: (error: Error) => void;
};

const errorValue = (value: unknown) =>
	value instanceof Error ? value : new Error(String(value));

/*
FluidManifoldFeed consumes native MeasurementT updates from signals.manifold
via the standard WebSocket stream.
*/
export class FluidManifoldFeed {
	private disposed = false;
	private unsubscribeOnline: (() => void) | null = null;
	private unsubscribeFocus: (() => void) | null = null;
	private unsubscribeManifold: (() => void) | null = null;

	constructor(private readonly handlers: FluidFeedHandlers) {}

	private processLatest() {
		if (this.disposed) {
			return;
		}

		const symbol = focusAtom.get();
		const manifoldStore = signals.manifold;
		if (!manifoldStore) {
			return;
		}

		const ring =
			manifoldStore.state[symbol] ??
			Object.values(manifoldStore.state)[0];
		if (!ring) {
			return;
		}

		const measurement = ring.getLast();
		if (!measurement) {
			return;
		}

		try {
			const { fields, particles, phase } =
				decodeManifoldMeasurement(measurement);
			if (fields.momRho.length > 0) {
				this.handlers.onFields(fields);
			}
			this.handlers.onParticles(particles);
			this.handlers.onPhase(phase);
			this.handlers.onState("connected");
		} catch (error) {
			const err = errorValue(error);
			this.handlers.onError(err);
			errorAtom.set({
				error: err.message,
				source: "FluidManifoldFeed",
				symbol,
			});
		}
	}

	connect() {
		if (this.disposed) {
			return;
		}

		this.handlers.onState(
			onlineAtom.get() === "ONLINE" ? "connected" : "connecting",
		);

		const onlineSub = onlineAtom.subscribe((status) => {
			if (this.disposed) {
				return;
			}

			if (status === "ONLINE") {
				this.handlers.onState("connected");
				this.processLatest();
				return;
			}

			if (status === "CONNECTING") {
				this.handlers.onState("connecting");
				return;
			}

			this.handlers.onState("disconnected");
		});
		this.unsubscribeOnline = () => onlineSub.unsubscribe();

		const focusSub = focusAtom.subscribe(() => {
			this.processLatest();
		});
		this.unsubscribeFocus = () => focusSub.unsubscribe();

		const manifoldSub = signals.manifold.subscribe(() => {
			this.processLatest();
		});
		this.unsubscribeManifold = () => manifoldSub.unsubscribe();

		this.processLatest();
	}

	close() {
		this.disposed = true;
		this.unsubscribeOnline?.();
		this.unsubscribeOnline = null;
		this.unsubscribeFocus?.();
		this.unsubscribeFocus = null;
		this.unsubscribeManifold?.();
		this.unsubscribeManifold = null;
		this.handlers.onState("disconnected");
	}
}

/** @deprecated Prefer FluidManifoldFeed. */
export class FluidWebRTCFeed extends FluidManifoldFeed {}
