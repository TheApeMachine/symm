import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import type { Size } from "./types";

/*
Icon is a closed set of single-stroke glyphs drawn on one 24x24 grid at one
weight, so a row of them reads as one family rather than as pasted artwork.

Adding a glyph means adding a path to GLYPHS — never an inline <svg> at a call
site, which is how the set drifted into three different stroke widths before.

Every glyph strokes currentColor, so tone comes from the text colour of whatever
it sits in. Nothing here sets a colour of its own.
*/

const SIZE_PX: Record<Size, number> = {
	xxs: 9,
	xs: 11,
	s: 13,
	m: 15,
	lg: 17,
	xl: 20,
	xxl: 24,
};

const GLYPHS = {
	search: (
		<>
			<circle cx="11" cy="11" r="7" />
			<path d="M21 21l-4.3-4.3" />
		</>
	),
	close: <path d="M6 6l12 12M18 6L6 18" />,
	dashboard: (
		<>
			<rect x="3" y="3" width="7" height="9" />
			<rect x="14" y="3" width="7" height="5" />
			<rect x="14" y="12" width="7" height="9" />
			<rect x="3" y="16" width="7" height="5" />
		</>
	),
	signal: <path d="M3 12h4l2 7 4-16 2 9h6" />,
	tree: (
		<>
			<circle cx="12" cy="4" r="2" />
			<circle cx="5" cy="20" r="2" />
			<circle cx="19" cy="20" r="2" />
			<path d="M12 6v5M12 11l-6 6M12 11l6 6" />
		</>
	),
	graph: (
		<>
			<circle cx="5" cy="12" r="2" />
			<circle cx="12" cy="5" r="2" />
			<circle cx="19" cy="12" r="2" />
			<circle cx="12" cy="19" r="2" />
			<path d="M6.7 10.9l3.6-4.8M13.7 6.1l3.6 4.8M17.3 13.1l-3.6 4.8M10.3 17.9l-3.6-4.8" />
		</>
	),
	journal: (
		<>
			<path d="M6 4h9l3 3v13H6z" />
			<path d="M15 4v3h3" />
			<path d="M8 11h8M8 15h8" />
		</>
	),
	scan: (
		<>
			<path d="M4 7V5a1 1 0 0 1 1-1h2M17 4h2a1 1 0 0 1 1 1v2M20 17v2a1 1 0 0 1-1 1h-2M7 20H5a1 1 0 0 1-1-1v-2" />
			<circle cx="12" cy="12" r="3.2" />
		</>
	),
	cortex: (
		<>
			<circle cx="5" cy="12" r="2" />
			<circle cx="19" cy="6" r="2" />
			<circle cx="19" cy="18" r="2" />
			<path d="M7 12h4M11 12l6-5M11 12l6 5" />
		</>
	),
	bars: (
		<>
			<path d="M3 3v18h18" />
			<rect x="7" y="11" width="3" height="7" />
			<rect x="12" y="7" width="3" height="11" />
			<rect x="17" y="4" width="3" height="14" />
		</>
	),
	lanes: (
		<>
			<path d="M4 7h16M4 12h10M4 17h13" />
		</>
	),
	grid: (
		<>
			<rect x="3" y="3" width="7" height="7" />
			<rect x="14" y="3" width="7" height="7" />
			<rect x="3" y="14" width="7" height="7" />
			<rect x="14" y="14" width="7" height="7" />
		</>
	),
	target: (
		<>
			<path d="M5 12h14" />
			<path d="M12 5v14" />
			<circle cx="12" cy="12" r="7" />
		</>
	),
	chevronDown: <path d="M6 9l6 6 6-6" />,
	chevronRight: <path d="M9 6l6 6-6 6" />,
	check: <path d="M4 12.5l5 5 11-11" />,
	/*
		The failure mark is this set's own signal trace, cut.

		A triangle-bang would be the obvious choice and is the wrong one here: it
		is borrowed from road signage, it says nothing about this system, and it is
		the one glyph a reader has already learned to skip. Every other glyph in
		this family describes the thing it names — signal is a waveform, bars is a
		chart, tree is a tree — so the failure mark describes a failure: the trace
		climbs, stops mid-rise, and the far side is flat and never joins it.

		It is deliberately two strokes, not three. The signal path cut into
		fragments reads as three unrelated marks once it is 13px tall; a single
		interrupted rise against a single flat line still reads as one instrument
		that stopped, which is the whole message.
	*/
	broken: (
		<>
			<path d="M3 12h4l2 7 2.4-9.6" />
			<path d="M15.5 12H21" />
		</>
	),
	spark: (
		<>
			<circle cx="12" cy="12" r="2.2" />
			<path d="M12 3v4.2M12 16.8V21M3 12h4.2M16.8 12H21M5.8 5.8l3 3M15.2 15.2l3 3M18.2 5.8l-3 3M8.8 15.2l-3 3" />
		</>
	),
} satisfies Record<string, ReactNode>;

export type IconName = keyof typeof GLYPHS;

export const ICON_NAMES = Object.keys(GLYPHS) as IconName[];

export type IconProps = Omit<ComponentProps<"svg">, "children" | "name"> & {
	name: IconName;
	size?: Size;
	/*
		An icon that sits beside its own label is decoration and stays out of the
		accessibility tree. One that stands alone — a bare close or search control —
		is the only label there is, so it takes a title.
	*/
	title?: string;
};

export const Icon = ({
	ref,
	name,
	size = "m",
	title,
	className,
	...props
}: IconProps) => {
	const px = SIZE_PX[size];

	return (
		<svg
			ref={ref}
			width={px}
			height={px}
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			strokeWidth="1.7"
			strokeLinecap="round"
			strokeLinejoin="round"
			className={cn("shrink-0", className)}
			{...(title === undefined
				? { "aria-hidden": true }
				: { role: "img", "aria-label": title })}
			{...props}
		>
			<title>{title ?? name}</title>
			{GLYPHS[name]}
		</svg>
	);
};
