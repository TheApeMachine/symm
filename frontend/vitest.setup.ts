/*
Node 26 exposes localStorage only when started with --localstorage-file, and its
absence shadows the one jsdom would otherwise provide, so a test running in the
jsdom environment finds `window` and `document` but no storage at all.

Anything persisted by the app — the pipeline graph collection, a remembered
routing mode — reads it at import time, so the gap surfaces as an undefined
property rather than as a missing feature. This installs an in-memory
implementation when, and only when, the environment has a window and no
storage on it. A test that wants to observe what was written can read it back
through the same object.
*/
const install = () => {
	if (typeof window === "undefined" || window.localStorage) {
		return;
	}

	const entries = new Map<string, string>();

	Object.defineProperty(window, "localStorage", {
		configurable: true,
		value: {
			get length() {
				return entries.size;
			},
			key: (index: number) => [...entries.keys()][index] ?? null,
			getItem: (key: string) => entries.get(key) ?? null,
			setItem: (key: string, value: string) => {
				entries.set(key, String(value));
			},
			removeItem: (key: string) => {
				entries.delete(key);
			},
			clear: () => {
				entries.clear();
			},
		},
	});
};

install();
