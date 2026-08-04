export type Variant =
	| "brand"
	| "info"
	| "success"
	| "warning"
	| "error"
	| "disabled";

export type Size = "xxs" | "xs" | "s" | "m" | "lg" | "xl" | "xxl";

/*
Tone is the full colour vocabulary: Variant plus the two the semantic set has no
name for — the terminal's own accent, and plain muted text. Components that only
carry state use Variant; components that also carry chrome use Tone.
*/
export type Tone = Variant | "accent" | "muted";

/*
SIZE_ORDER lets a component step a size up or down without carrying its own
lookup table — a dot inside a badge is one step smaller than the badge itself.
*/
export const SIZE_ORDER: readonly Size[] = [
	"xxs",
	"xs",
	"s",
	"m",
	"lg",
	"xl",
	"xxl",
];

export const stepSize = (size: Size, by: number): Size => {
	const index = SIZE_ORDER.indexOf(size) + by;

	return (
		SIZE_ORDER[Math.min(SIZE_ORDER.length - 1, Math.max(0, index))] ?? size
	);
};
