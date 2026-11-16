import { atoms, signals } from "#/collections/app";
import { ingestPathWalks } from "#/collections/cognition";
import type { Decision, PathToken, Paths } from "#/collections/envelope";

/*
pathSegments mirrors nomagique runtime.Token.String: one symbol or [a-b-c].
*/
export const pathSegments = (cut: unknown): string[] => {
	if (!Array.isArray(cut)) return [];

	const out: string[] = [];

	for (const raw of cut) {
		if (!raw || typeof raw !== "object") continue;
		const symbols = (raw as PathToken).symbols;
		if (!Array.isArray(symbols) || symbols.length === 0) continue;
		if (symbols.length === 1) {
			const lone = symbols[0];
			if (lone !== undefined) {
				out.push(lone);
			}
		} else {
			out.push(`[${symbols.join("-")}]`);
		}
	}

	return out;
};

const decisionForSymbol = (
	decisions: Record<string, Decision>,
	symbol: string,
): Decision | undefined => {
	const direct = decisions[symbol];
	if (direct?.symbol === symbol) return direct;

	for (const decision of Object.values(decisions)) {
		if (decision?.symbol === symbol) return decision;
	}

	return undefined;
};

export const ingestLivePaths = (
	tick: number,
	paths: Paths,
	decisions: Record<string, Decision>,
): boolean => {
	const rows = [];

	for (const [symbol, cut] of Object.entries(paths)) {
		const tokens = pathSegments(cut);
		if (tokens.length === 0) continue;

		const phase = tokens.join("/");
		const decision = decisionForSymbol(decisions, symbol);
		const action = decision?.action;

		rows.push({
			symbol,
			phase,
			policy: phase,
			storyPhase:
				action === "enter" || action === "hold" || action === "exit"
					? action
					: undefined,
			enter: action === "enter",
		});
	}

	if (rows.length === 0) return false;
	return ingestPathWalks(tick, rows);
};

const syncLivePaths = () => {
	const tick = atoms.tick.get()?.n;
	if (tick == null || tick <= 0) return;
	ingestLivePaths(tick, signals.paths.state, signals.decisions.state);
};

signals.paths.subscribe(syncLivePaths);
signals.decisions.subscribe(syncLivePaths);
atoms.tick.subscribe(syncLivePaths);