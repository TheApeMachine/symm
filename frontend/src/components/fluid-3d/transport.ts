import {
	decodeManifold,
	type FluidFields,
	type FluidParticleFrame,
	type FluidPhase,
} from "./wire";
import { onlineAtom } from "#/collections/app";
import { subscribeManifold } from "#/providers/websocket";

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
FluidManifoldFeed consumes ManifoldFrame payloads from the hub WebSocket
(uiTee → /ws). It does not open WebRTC or POST /webrtc/manifold.
*/
export class FluidManifoldFeed {
	private disposed = false;
	private unsubscribe: (() => void) | null = null;
	private unsubscribeOnline: (() => void) | null = null;

	constructor(private readonly handlers: FluidFeedHandlers) {}

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
				return;
			}

			if (status === "CONNECTING") {
				this.handlers.onState("connecting");
				return;
			}

			this.handlers.onState("disconnected");
		});
		this.unsubscribeOnline = () => onlineSub.unsubscribe();

		this.unsubscribe = subscribeManifold((bytes) => {
			if (this.disposed) {
				return;
			}

			try {
				const { fields, particles, phase } = decodeManifold(bytes);
				this.handlers.onFields(fields);
				this.handlers.onParticles(particles);
				this.handlers.onPhase(phase);
				this.handlers.onState("connected");
			} catch (error) {
				this.handlers.onError(errorValue(error));
			}
		});
	}

	close() {
		this.disposed = true;
		this.unsubscribe?.();
		this.unsubscribe = null;
		this.unsubscribeOnline?.();
		this.unsubscribeOnline = null;
		this.handlers.onState("disconnected");
	}
}

/** @deprecated Prefer FluidManifoldFeed. */
export class FluidWebRTCFeed extends FluidManifoldFeed {}
