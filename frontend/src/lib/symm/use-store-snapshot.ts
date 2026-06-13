import { useSyncExternalStore } from "react";

type SnapshotStore<T> = {
	subscribe: (listener: () => void) => () => void;
	snapshot: () => T;
};

export const useStoreSnapshot = <T>(store: SnapshotStore<T>): T =>
	useSyncExternalStore(store.subscribe, store.snapshot, store.snapshot);
