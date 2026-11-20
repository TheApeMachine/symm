export const kernelRowAt = <T>(
	state: Record<string, T>,
	symbol: string,
): T | undefined => state[symbol] ?? state[symbol.replace(/\//g, "-")];