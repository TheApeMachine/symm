/*
isUngatedThreshold detects DMT's ungated entropy sentinel (MaxFloat64) so the
UI does not paint Number.MAX_VALUE as a real gate.
*/
export const isUngatedThreshold = (threshold: number): boolean =>
	threshold === Number.POSITIVE_INFINITY ||
	threshold >= Number.MAX_VALUE / 2;

/*
formatEntropyGate renders bits against a real threshold, or "ungated" when DMT
reported that branching entropy does not apply.
*/
export const formatEntropyGate = (
	bits: number,
	threshold: number,
): { value: string; percent: number; ungated: boolean } => {
	const safeBits = Number.isFinite(bits) ? bits : 0;

	if (isUngatedThreshold(threshold)) {
		return {
			value: `${safeBits.toFixed(2)} / ungated`,
			percent: 0,
			ungated: true,
		};
	}

	const safeThreshold = Number.isFinite(threshold) ? threshold : 0;
	const percent =
		safeThreshold > 0
			? Math.min(100, Math.max(0, (safeBits / safeThreshold) * 100))
			: 0;

	return {
		value: `${safeBits.toFixed(2)} / ${safeThreshold.toFixed(2)} bits`,
		percent,
		ungated: false,
	};
};

/*
formatBeamSequence tokenizes a DMT underscore sequence for readable preview
while keeping the full wire string available as a title tooltip.
*/
export const formatBeamSequence = (
	sequence: string,
	maxTokens = 8,
): { preview: string; title: string } => {
	const raw = sequence.trim();

	if (raw === "" || raw === "waiting") {
		return { preview: "waiting", title: "waiting" };
	}

	const tokens = raw.split("_").filter(Boolean);

	if (tokens.length === 0) {
		return { preview: raw, title: raw };
	}

	const visible =
		tokens.length <= maxTokens
			? tokens
			: tokens.slice(Math.max(0, tokens.length - maxTokens));
	const preview =
		tokens.length <= maxTokens
			? visible.join(" · ")
			: `… · ${visible.join(" · ")}`;

	return { preview, title: raw };
};
