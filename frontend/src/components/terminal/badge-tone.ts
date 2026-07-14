import type { Variant } from "@/components/ui/types";

/*
verdictToVariant maps backend decision verdicts onto badge variants.
*/
export const verdictToVariant = (verdict: string): Variant => {
	if (verdict === "allow") {
		return "success";
	}

	if (verdict === "blocked") {
		return "error";
	}

	return "info";
};

/*
verdictBadgeClassName styles non-terminal SYMM verdict states.
*/
export const verdictBadgeClassName = (verdict: string): string | undefined =>
	verdict === "allow" || verdict === "blocked"
		? undefined
		: "border-(--line) bg-(--line) [--badge-tone:var(--f3)]";

/*
paletteGroupVariant maps palette command groups onto badge variants.
*/
export const paletteGroupVariant = (group: string): Variant => {
	if (group === "Surface") {
		return "info";
	}

	if (group === "Symbol") {
		return "success";
	}

	return "warning";
};
