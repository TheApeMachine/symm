import type { Variant } from "@/components/ui/types";

export const paletteGroupVariant = (group: string): Variant => {
	if (group === "Surface") {
		return "info";
	}

	if (group === "Symbol") {
		return "success";
	}

	return "warning";
};