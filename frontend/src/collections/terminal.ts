import { createStore } from "@tanstack/react-store";
import { DEFAULT_FOCUS_SYMBOL } from "#/collections/app";

export type TerminalSurface =
	| "dashboard"
	| "graph"
	| "influence"
	| "lineage"
	| "fluid"
	| "signals"
	| "decisions"
	| "journal"
	| "xray"
	| "cortex"
	| "allocation"
	| "regulator"
	| "diagnostics"
	| "backtest";

export { DEFAULT_FOCUS_SYMBOL };

export const terminalStore = createStore(
	{
		scanlines: true,
		/*
			No kernel is selected until one is picked or the rail resolves the first
			live source. The default used to name "manifold", which is a surface of
			its own and never appears as a measurement source, so the detail panel
			pinned every binding to a row the wire cannot send and opened blank.
		*/
		selectedSource: "",
		inspectorSource: null as string | null,
		paletteOpen: false,
		paletteMode: "all" as "all" | "symbols",
		paletteQuery: "",
		paletteIndex: 0,
		focusSymbol: DEFAULT_FOCUS_SYMBOL,
		thesisSymbol: null as string | null,
	},
	({ setState }) => ({
		toggleScanlines: () =>
			setState((prev) => ({
				...prev,
				scanlines: !prev.scanlines,
			})),
		selectSource: (selectedSource: string) =>
			setState((prev) => ({
				...prev,
				selectedSource,
			})),
		inspectSource: (source: string) =>
			setState((prev) => ({
				...prev,
				selectedSource: source,
				inspectorSource: source,
			})),
		closeInspect: () =>
			setState((prev) => ({
				...prev,
				inspectorSource: null,
			})),
		openPalette: () =>
			setState((prev) => ({
				...prev,
				paletteOpen: true,
				paletteMode: "all",
				paletteQuery: "",
				paletteIndex: 0,
			})),
		openSymbolPalette: () =>
			setState((prev) => ({
				...prev,
				paletteOpen: true,
				paletteMode: "symbols",
				paletteQuery: "",
				paletteIndex: 0,
			})),
		closePalette: () =>
			setState((prev) => ({
				...prev,
				paletteOpen: false,
			})),
		setPaletteQuery: (paletteQuery: string) =>
			setState((prev) => ({
				...prev,
				paletteQuery,
				paletteIndex: 0,
			})),
		bumpPaletteIndex: (delta: number) =>
			setState((prev) => ({
				...prev,
				paletteIndex: prev.paletteIndex + delta,
			})),
		selectFocusSymbol: (focusSymbol: string) =>
			setState((prev) => ({
				...prev,
				focusSymbol,
			})),
		openThesis: (thesisSymbol: string) =>
			setState((prev) => ({
				...prev,
				thesisSymbol,
			})),
		closeThesis: () =>
			setState((prev) => ({
				...prev,
				thesisSymbol: null,
			})),
	}),
);
