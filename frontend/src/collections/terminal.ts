import { createStore } from "@tanstack/react-store";
import { DEFAULT_FOCUS_SYMBOL } from "#/collections/app";

export type TerminalSurface =
	| "dashboard"
	| "learning"
	| "influence"
	| "lineage"
	| "fluid"
	| "signals"
	| "journal"
	| "xray"
	| "cortex"
	| "allocation"
	| "diagnostics"
	| "hindsight"
	| "workbench"
	| "pipeline";

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
			setState((prev: any) => ({
				...prev,
				scanlines: !prev.scanlines,
			})),
		selectSource: (selectedSource: string) =>
			setState((prev: any) => ({
				...prev,
				selectedSource,
			})),
		inspectSource: (source: string) =>
			setState((prev: any) => ({
				...prev,
				selectedSource: source,
				inspectorSource: source,
			})),
		closeInspect: () =>
			setState((prev: any) => ({
				...prev,
				inspectorSource: null,
			})),
		openPalette: () =>
			setState((prev: any) => ({
				...prev,
				paletteOpen: true,
				paletteMode: "all",
				paletteQuery: "",
				paletteIndex: 0,
			})),
		openSymbolPalette: () =>
			setState((prev: any) => ({
				...prev,
				paletteOpen: true,
				paletteMode: "symbols",
				paletteQuery: "",
				paletteIndex: 0,
			})),
		closePalette: () =>
			setState((prev: any) => ({
				...prev,
				paletteOpen: false,
			})),
		setPaletteQuery: (paletteQuery: string) =>
			setState((prev: any) => ({
				...prev,
				paletteQuery,
				paletteIndex: 0,
			})),
		bumpPaletteIndex: (delta: number) =>
			setState((prev: any) => ({
				...prev,
				paletteIndex: prev.paletteIndex + delta,
			})),
		selectFocusSymbol: (focusSymbol: string) =>
			setState((prev: any) => ({
				...prev,
				focusSymbol,
			})),
		openThesis: (thesisSymbol: string) =>
			setState((prev: any) => ({
				...prev,
				thesisSymbol,
			})),
		closeThesis: () =>
			setState((prev: any) => ({
				...prev,
				thesisSymbol: null,
			})),
	}),
);
