/*
TelemetryBus is a zero-allocation PubSub event bus for high-frequency canvas
visualization. It routes transferred Float32Array frame buffers directly from
worker ingestion to canvas render contexts, completely bypassing React Virtual
DOM reconciliation and eliminating V8 GC object allocation overhead.
*/

export type TypedTelemetryChannel = "phase_dial" | "prediction_scalars" | "oscillators";

export type TypedFrameEvent = {
	channel: TypedTelemetryChannel;
	symbol: string;
	data: Float32Array;
};

type FrameListener = (event: TypedFrameEvent) => void;

class TelemetryBus {
	private readonly listeners = new Map<TypedTelemetryChannel, Set<FrameListener>>();

	subscribe(channel: TypedTelemetryChannel, listener: FrameListener): () => void {
		let set = this.listeners.get(channel);

		if (!set) {
			set = new Set();
			this.listeners.set(channel, set);
		}

		set.add(listener);

		return () => {
			set?.delete(listener);
		};
	}

	emit(event: TypedFrameEvent) {
		const set = this.listeners.get(event.channel);

		if (!set || set.size === 0) {
			return;
		}

		for (const listener of set) {
			listener(event);
		}
	}
}

export const telemetryBus = new TelemetryBus();
