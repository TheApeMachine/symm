import {
	decodeManifold,
	type FluidFields,
	type FluidParticleFrame,
	type FluidPhase,
} from "./wire";
import { atoms, signals } from "#/collections/app";

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
FluidManifoldFeed consumes manifold Values from signals.manifold
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

		const symbol = atoms["focus"].get();
		const manifoldStore = signals["manifold"];
		if (!manifoldStore) {
			return;
		}

		const ring =
			manifoldStore.state[symbol] ??
			Object.values(manifoldStore.state)[0];
		if (!ring) {
			return;
		}

		const value = ring.getLast();
		if (!value) {
			return;
		}

		try {
			const { fields, particles, phase } = decodeManifold(value);
			if (fields.momRho.length > 0) {
				this.handlers.onFields(fields);
			}
			this.handlers.onParticles(particles);
			this.handlers.onPhase(phase);
			this.handlers.onState("connected");
		} catch (error) {
			const err = errorValue(error);
			this.handlers.onError(err);
			atoms["error"].set({
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
			atoms["online"].get() === "ONLINE" ? "connected" : "connecting",
		);

		const onlineSub = atoms["online"].subscribe((status) => {
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

		const focusSub = atoms["focus"].subscribe(() => {
			this.processLatest();
		});
		this.unsubscribeFocus = () => focusSub.unsubscribe();

		const manifoldSub = signals["manifold"].subscribe(() => {
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
