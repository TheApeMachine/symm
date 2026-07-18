import { createStore } from "@tanstack/react-store";
import { appStore, DEFAULT_FOCUS_SYMBOL } from "./app";

const symbolFrom = (value: unknown): string => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		return "";
	}

	const record = value as Record<string, unknown>;
	const symbol = String(record.symbol ?? record.scope ?? "").trim();

	return symbol.includes("/") ? symbol : "";
};

const pairsFrom = (frame: Record<string, unknown> | unknown[]) => {
	if (Array.isArray(frame)) {
		return frame;
	}

	const data =
		frame.data !== null && typeof frame.data === "object"
			? (frame.data as Record<string, unknown>)
			: {};

	if (Array.isArray(data.pairs)) {
		return data.pairs;
	}

	if (Array.isArray(frame.pairs)) {
		return frame.pairs;
	}

	if (Array.isArray(frame.symbols)) {
		return frame.symbols;
	}

	return [frame];
};

export const instrumentsStore = createStore(
	{
		frame: null as Record<string, unknown> | unknown[] | null,
		instruments: {} as Record<string, Record<string, unknown>>,
		symbols: [] as string[],
	},
	({ setState }) => ({
		updateFrame: (frame: Record<string, unknown> | unknown[]) =>
			setState((prev) => {
				const instruments = { ...prev.instruments };

				for (const pair of pairsFrom(frame)) {
					const symbol = symbolFrom(pair);

					if (symbol !== "") {
						instruments[symbol] = pair as Record<string, unknown>;
					}
				}

				const symbols = Object.keys(instruments).sort();
				const focus = appStore.state.focusSymbol;

				// Keep an explicit focus. Only adopt the default major when the
				// current focus is empty; never jump to a lexical first pair.
				if (focus === "" && symbols.includes(DEFAULT_FOCUS_SYMBOL)) {
					appStore.actions.updateFocusSymbol(DEFAULT_FOCUS_SYMBOL);
				}

				return {
					frame,
					instruments,
					symbols,
				};
			}),
	}),
);
