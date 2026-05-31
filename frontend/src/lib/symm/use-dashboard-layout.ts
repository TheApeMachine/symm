import { useSyncExternalStore } from "react";

import {
	defaultLayoutDocument,
	type LayoutDocument,
} from "#/lib/symm/layout-schema";
import { LayoutStore } from "#/lib/symm/layout-store";

const subscribeLayout = (listener: () => void) =>
	LayoutStore.subscribe(() => listener());

const getLayoutSnapshot = (): LayoutDocument => LayoutStore.snapshot();

export const useDashboardLayout = (): LayoutDocument =>
	useSyncExternalStore(
		subscribeLayout,
		getLayoutSnapshot,
		defaultLayoutDocument,
	);
