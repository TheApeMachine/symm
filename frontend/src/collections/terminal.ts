import { createStore } from "@tanstack/react-store";

export type TerminalSurface =
	| "dashboard"
	| "signals"
	| "decisions"
	| "journal"
	| "xray"
	| "cortex"
	| "allocation";

export const DEFAULT_FOCUS_SYMBOL = "stream";

export const terminalStore = createStore(
	{
		scanlines: true,
		fieldStyle: "Heatmap" as "Heatmap" | "Contour",
		selectedSource: "fluid",
		inspectorSource: null as string | null,
		paletteOpen: false,
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
		toggleFieldStyle: () =>
			setState((prev) => ({
				...prev,
				fieldStyle: prev.fieldStyle === "Heatmap" ? "Contour" : "Heatmap",
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
