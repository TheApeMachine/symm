const QUOTE_CURRENCIES = "USD|EUR|USDT|USDC|BTC|ETH|GBP|CAD|AUD|JPY|CHF";
const SYMBOL_PAIR_TEXT = `\\b[A-Za-z0-9][A-Za-z0-9._-]{0,19}\\/(?:${QUOTE_CURRENCIES})\\b`;
const SYMBOL_PAIR_PATTERN = new RegExp(SYMBOL_PAIR_TEXT, "gi");
const SYMBOL_PAIR_EXACT = new RegExp(
	`^[A-Za-z0-9][A-Za-z0-9._-]{0,19}\\/(?:${QUOTE_CURRENCIES})$`,
	"i",
);

export const normalizeSymbolPair = (value: string): string =>
	value.trim().toUpperCase();

export const isSymbolPair = (value: unknown): value is string =>
	typeof value === "string" &&
	SYMBOL_PAIR_EXACT.test(normalizeSymbolPair(value));

export const symbolPairsFromText = (value: unknown): string[] => {
	if (typeof value !== "string") {
		return [];
	}

	const matches = value.match(SYMBOL_PAIR_PATTERN) ?? [];

	return [...new Set(matches.map(normalizeSymbolPair).filter(isSymbolPair))];
};

export const symbolPairFromText = (value: unknown): string | null => {
	const symbols = symbolPairsFromText(value);

	return symbols.length === 1 ? (symbols[0] ?? null) : null;
};

export const collectSymbolPairs = (value: unknown, limit = 1500): string[] => {
	const symbols = new Set<string>();
	const seen = new WeakSet<object>();
	let visited = 0;

	const visit = (candidate: unknown) => {
		if (visited >= limit || candidate === null || candidate === undefined) {
			return;
		}

		visited += 1;

		if (typeof candidate === "string") {
			for (const symbol of symbolPairsFromText(candidate)) {
				symbols.add(symbol);
			}
			return;
		}

		if (typeof candidate !== "object") {
			return;
		}

		if (seen.has(candidate)) {
			return;
		}
		seen.add(candidate);

		if (Array.isArray(candidate)) {
			for (const item of candidate) {
				visit(item);
			}
			return;
		}

		for (const [key, item] of Object.entries(
			candidate as Record<string, unknown>,
		)) {
			visit(key);
			visit(item);
		}
	};

	visit(value);

	return [...symbols].sort((left, right) => left.localeCompare(right));
};

export const symbolsFromReadings = (
	readings: Record<string, Record<string, unknown>>,
): string[] => {
	const symbols = new Set<string>();

	for (const bySymbol of Object.values(readings)) {
		for (const symbol of Object.keys(bySymbol)) {
			if (isSymbolPair(symbol)) {
				symbols.add(normalizeSymbolPair(symbol));
			}
		}
	}

	return [...symbols].sort((left, right) => left.localeCompare(right));
};
