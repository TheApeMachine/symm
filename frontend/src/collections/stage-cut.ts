import { symbolScopeAliases, workspaceFrame } from "#/collections/app";
import type { Value } from "#/types/value";

type WireToken = {
	symbols?: string[];
	brightness?: number;
};

const tokenLabel = (token: WireToken): string => {
	const symbols = token.symbols ?? [];
	if (symbols.length === 0) return "";
	if (symbols.length === 1) return symbols[0];
	return `[${symbols.join("-")}]`;
};

const parseCut = (payload: unknown): WireToken[] => {
	if (!Array.isArray(payload)) return [];
	return payload.filter(
		(entry): entry is WireToken =>
			entry != null && typeof entry === "object" && !Array.isArray(entry),
	);
};

const cutLayer = (frame: Value, layerKey: string): WireToken[] => {
	const direct = parseCut(frame.reading(layerKey));
	if (direct.length > 0) return direct;

	for (const { payload } of frame.layersUnder(layerKey)) {
		const tokens = parseCut(payload);
		if (tokens.length > 0) return tokens;
	}

	return [];
};

/*
stageCutReading reads the live sequencer cut from the workspace aggregate tick.
*/
export const stageCutReading = (
	symbol: string,
): { fired: string[]; midpoint: number; path: string } => {
	const aggregate = workspaceFrame();
	if (!aggregate) {
		return { fired: [], midpoint: 0, path: "" };
	}

	for (const alias of symbolScopeAliases(symbol)) {
		const tokens = cutLayer(aggregate, `stage/${alias}/cut`);
		if (tokens.length === 0) continue;

		const last = tokens[tokens.length - 1];
		const fired = [...(last.symbols ?? [])];
		const midpoint =
			typeof last.brightness === "number" ? last.brightness : 0;
		const path = tokens
			.map(tokenLabel)
			.filter((segment) => segment !== "")
			.join("/");

		return { fired, midpoint, path };
	}

	return { fired: [], midpoint: 0, path: "" };
};
