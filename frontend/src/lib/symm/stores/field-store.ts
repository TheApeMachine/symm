import { createStore } from "@tanstack/react-store";

import type { FieldSnapshotEvent } from "#/lib/symm/events";

export type FieldStoreState = {
	fieldSnapshot?: FieldSnapshotEvent;
};

export const fieldStore = createStore<FieldStoreState>({});
