import {
	defaultLayoutDocument,
	isLayoutDocument,
	normalizeLayoutDocument,
	type LayoutDocument,
} from "#/lib/symm/layout-schema";

type LayoutListener = (layout: LayoutDocument) => void;

/*
LayoutStore holds the backend-provided dashboard schema and notifies layout renderers.
*/
class LayoutStoreImpl {
	private layout: LayoutDocument = defaultLayoutDocument();
	private listeners = new Set<LayoutListener>();

	snapshot(): LayoutDocument {
		return this.layout;
	}

	subscribe(listener: LayoutListener) {
		this.listeners.add(listener);
		listener(this.layout);

		return () => {
			this.listeners.delete(listener);
		};
	}

	ingest(raw: unknown) {
		if (!isLayoutDocument(raw)) {
			return;
		}

		this.layout = normalizeLayoutDocument(raw);

		for (const listener of this.listeners) {
			listener(this.layout);
		}
	}

	reset() {
		this.layout = defaultLayoutDocument();

		for (const listener of this.listeners) {
			listener(this.layout);
		}
	}
}

export const LayoutStore = new LayoutStoreImpl();

export const createLayoutStore = () => new LayoutStoreImpl();
