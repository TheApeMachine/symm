import { createStore } from "@tanstack/react-store";

import type { FieldSnapshotEvent, FluidDisplayEvent } from "#/lib/symm/events";

export type FieldStoreState = {
	fieldSnapshot?: FieldSnapshotEvent;
	fluidDisplay?: FluidDisplayEvent;
};

export const fieldStore = createStore<FieldStoreState>({});

export const applyFluidDisplay = (display: FluidDisplayEvent) => {
	fieldStore.setState((state) => ({ ...state, fluidDisplay: display }));
};
