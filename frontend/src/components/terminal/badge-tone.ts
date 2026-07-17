import type { Variant } from "@/components/ui/types";

/*
verdictToVariant maps strategy/candidate verdicts onto badge variants.
*/
export const verdictToVariant = (verdict: string): Variant => {
	switch (verdict) {
		case "allow":
			return "success";
		case "blocked":
			return "error";
		case "hold":
			return "warning";
		case "waiting":
		case "below":
			return "info";
		default: {
			const _exhaustive: string = verdict;
			return _exhaustive.length > 0 ? "disabled" : "disabled";
		}
	}
};

/*
verdictBadgeClassName styles non-terminal SYMM verdict states.
*/
export const verdictBadgeClassName = (verdict: string): string | undefined =>
	verdict === "allow" || verdict === "blocked" || verdict === "hold"
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
