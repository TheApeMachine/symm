export const symbolScopeAliases = (symbol: string): string[] => {
	if (!symbol) return [];
	const aliases = new Set<string>([symbol]);
	if (symbol.includes("/")) {
		aliases.add(symbol.replace(/\//g, "-"));
	} else {
		const dash = symbol.indexOf("-");
		if (dash > 0) {
			aliases.add(`${symbol.slice(0, dash)}/${symbol.slice(dash + 1)}`);
		}
	}
	return [...aliases];
};