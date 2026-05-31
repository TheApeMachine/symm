import type { DecisionTraceEvent } from "#/lib/symm/events";
import { isDecisionTraceEvent } from "#/lib/symm/events";

type Listener = () => void;

class DecisionsDataProviderImpl {
	private latest: DecisionTraceEvent | undefined;
	private listeners = new Set<Listener>();

	subscribe(listener: Listener) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): DecisionTraceEvent | undefined {
		return this.latest;
	}

	private notify() {
		for (const listener of this.listeners) {
			listener();
		}
	}

	ingest(raw: unknown) {
		if (!isDecisionTraceEvent(raw)) {
			return;
		}

		this.latest = raw;
		this.notify();
	}

	reset() {
		this.latest = undefined;
		this.notify();
	}
}

const shared = createDecisionsDataProviderImpl();

export const createDecisionsDataProvider = () =>
	createDecisionsDataProviderImpl();

function createDecisionsDataProviderImpl() {
	const impl = new DecisionsDataProviderImpl();

	return {
		subscribe: (listener: Listener) => impl.subscribe(listener),
		snapshot: () => impl.snapshot(),
		ingest: (raw: unknown) => impl.ingest(raw),
		reset: () => impl.reset(),
	};
}

export type DecisionsStore = ReturnType<typeof createDecisionsDataProvider>;

export const DecisionsDataProvider = shared;
