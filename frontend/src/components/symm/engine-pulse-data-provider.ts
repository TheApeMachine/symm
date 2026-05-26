import type { EnginePulseEvent } from "#/lib/symm/events";
import { isEnginePulseEvent } from "#/lib/symm/events";

type PulseSink = (pulse: EnginePulseEvent) => void;
type Listener = () => void;

/*
EnginePulseDataProvider routes engine_pulse ticks to the prediction chart.
*/
class EnginePulseDataProviderImpl {
	private sink: PulseSink | null = null;
	private latest: EnginePulseEvent | undefined;
	private listeners = new Set<Listener>();

	registerSink(sink: PulseSink) {
		this.sink = sink;

		if (this.latest !== undefined) {
			sink(this.latest);
		}

		return () => {
			if (this.sink === sink) {
				this.sink = null;
			}
		};
	}

	subscribe(listener: Listener) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): EnginePulseEvent | undefined {
		return this.latest;
	}

	private notify() {
		for (const listener of this.listeners) {
			listener();
		}
	}

	ingest(raw: unknown) {
		if (!isEnginePulseEvent(raw)) {
			return;
		}

		this.latest = raw;
		this.sink?.(raw);
		this.notify();
	}
}

const shared = new EnginePulseDataProviderImpl();

export const EnginePulseDataProvider = {
	registerSink: (sink: PulseSink) => shared.registerSink(sink),
	subscribe: (listener: Listener) => shared.subscribe(listener),
	snapshot: () => shared.snapshot(),
	ingest: (raw: unknown) => shared.ingest(raw),
};
