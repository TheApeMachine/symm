import { type HTMLMotionProps, motion } from "motion/react";
import type { CSSProperties, ReactNode } from "react";
import { cn } from "@/lib/utils";

const colsClasses = Array.from(
	{ length: 12 },
	(_, i) => `grid-cols-${i + 1}`,
).concat("grid-cols-none");

const rowsClasses = Array.from(
	{ length: 6 },
	(_, i) => `grid-rows-${i + 1}`,
).concat("grid-rows-none");

const gapClasses = Array.from({ length: 16 }, (_, i) => `gap-${i}`).concat(
	"gap-0",
);

const padClasses = Array.from({ length: 16 }, (_, i) => `p-${i}`).concat("p-0");

const flowClasses = {
	col: "grid-flow-col",
	"col-dense": "grid-flow-col-dense",
	dense: "grid-flow-dense",
	row: "grid-flow-row",
	"row-dense": "grid-flow-row-dense",
} as const;

const alignClasses = {
	baseline: "items-baseline",
	center: "items-center",
	end: "items-end",
	start: "items-start",
	stretch: "items-stretch",
} as const;

const justifyClasses = {
	center: "justify-items-center",
	end: "justify-items-end",
	start: "justify-items-start",
	stretch: "justify-items-stretch",
} as const;

const placeClasses = {
	around: "place-content-around",
	between: "place-content-between",
	center: "place-content-center",
	end: "place-content-end",
	evenly: "place-content-evenly",
	start: "place-content-start",
	stretch: "place-content-stretch",
} as const;

export type SegmentsType = 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12;
type PadType = keyof typeof padClasses;
type GapType = keyof typeof gapClasses;
type FlowType = keyof typeof flowClasses;
type AlignType = keyof typeof alignClasses;
type JustifyType = keyof typeof justifyClasses;
type PlaceType = keyof typeof placeClasses;

type GridProps = Omit<HTMLMotionProps<"div">, "children"> & {
	cols?: SegmentsType;
	rows?: SegmentsType;
	pad?: PadType;
	gap?: GapType;
	gapX?: GapType;
	gapY?: GapType;
	flow?: FlowType;
	align?: AlignType;
	justify?: JustifyType;
	place?: PlaceType;
	fullWidth?: boolean;
	fullHeight?: boolean;
	style?: CSSProperties;
	responsive?: boolean;
	autoRows?: CSSProperties["gridAutoRows"];
	children?: ReactNode;
};

export type IslandAreaKey = "h" | "f" | "n" | "a" | "m";

export const ISLAND_GRID_CLASS =
	"[grid-template-areas:'h_h_h'_'n_m_a'_'f_f_f'] grid-cols-[auto_1fr_auto] grid-rows-[auto_1fr_auto]";

export const ISLAND_AREA_CLASS: Record<IslandAreaKey, string> = {
	h: "[grid-area:h]",
	f: "[grid-area:f]",
	n: "[grid-area:n]",
	a: "[grid-area:a]",
	m: "[grid-area:m]",
};

// Base Grid component
export const Grid = ({
	children,
	className,
	cols = 6,
	rows,
	pad,
	gap,
	gapX,
	gapY,
	flow = "row",
	align = "stretch",
	justify = "stretch",
	place,
	fullWidth = false,
	fullHeight = false,
	style,
	responsive = true,
	autoRows,
	...props
}: GridProps) => {
	// Generate responsive classes when responsive is true
	const getResponsiveColsClass = () => {
		if (!responsive || !cols) return cols ? colsClasses[cols - 1] : undefined;

		// Responsive approach: mobile -> tablet -> desktop
		// This ensures items display in a grid at reasonable breakpoints
		if (cols <= 2) {
			return "grid-cols-1 md:grid-cols-2";
		}
		if (cols <= 3) {
			return "grid-cols-1 md:grid-cols-2 lg:grid-cols-3";
		}
		if (cols <= 4) {
			return "grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4";
		}
		if (cols <= 6) {
			return "grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6";
		}
		// For cols > 6
		return "grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6";
	};

	return (
		<motion.div
			className={cn(
				"grid",
				"@container",
				getResponsiveColsClass(),
				rows ? rowsClasses[rows - 1] : undefined,
				gap && gapClasses[gap],
				pad && padClasses[pad],
				fullWidth && "w-full",
				fullHeight && "h-full",
				// Make grid items stretch horizontally to fill available space
				"*:min-w-0",
				"*:w-full",
				className,
			)}
			style={style}
			{...props}
		>
			{children}
		</motion.div>
	);
};

type GridSpanProps = Omit<HTMLMotionProps<"div">, "children"> & {
	cols: SegmentsType;
	rows: SegmentsType;
	colStart?: number;
	rowStart?: number;
	className?: string;
	children: ReactNode;
};

Grid.Span = ({
	cols,
	rows,
	colStart,
	rowStart,
	className,
	children,
	style,
	...rest
}: GridSpanProps) => {
	return (
		<motion.div
			{...rest}
			className={cn(
				"flex flex-col h-full",
				// Responsive span classes - mobile (2 cols) -> tablet (4/6 cols) -> desktop (6/12 cols)
				cols === 1 && "col-span-1",
				cols === 2 && "col-span-2 md:col-span-2 lg:col-span-2",
				cols === 3 && "col-span-2 md:col-span-3 lg:col-span-3",
				cols === 4 && "col-span-2 md:col-span-4 lg:col-span-4",
				cols === 5 && "col-span-2 md:col-span-4 lg:col-span-5",
				cols === 6 && "col-span-2 md:col-span-4 lg:col-span-6",
				cols === 7 && "col-span-2 md:col-span-4 lg:col-span-7",
				cols === 8 && "col-span-2 md:col-span-4 lg:col-span-8",
				cols === 9 && "col-span-2 md:col-span-6 lg:col-span-9",
				cols === 10 && "col-span-2 md:col-span-6 lg:col-span-10",
				cols === 11 && "col-span-2 md:col-span-6 lg:col-span-11",
				cols === 12 && "col-span-2 md:col-span-6 lg:col-span-12",
				rows === 1 && "row-span-1",
				rows === 2 && "row-span-2",
				rows === 3 && "row-span-3",
				rows === 4 && "row-span-4",
				rows === 5 && "row-span-5",
				rows === 6 && "row-span-6",
				className,
			)}
			style={{
				minHeight: 0,
				minWidth: 0,
				...style,
			}}
		>
			{children}
		</motion.div>
	);
};

Grid.IslandArea = ({
	area,
	className,
	children,
	...props
}: Omit<HTMLMotionProps<"div">, "children"> & {
	area: IslandAreaKey;
	children: ReactNode;
}) => {
	return (
		<motion.div
			className={cn("min-h-0 min-w-0", ISLAND_AREA_CLASS[area], className)}
			{...props}
		>
			{children}
		</motion.div>
	);
};

Grid.Island = ({
	className,
	children,
	...props
}: Omit<GridProps, "cols" | "rows" | "responsive">) => {
	return (
		<Grid
			responsive={false}
			className={cn(ISLAND_GRID_CLASS, className)}
			{...props}
		>
			{children}
		</Grid>
	);
};

// Responsive grid that adjusts columns based on min item width
// Uses auto-fit to ensure items stretch to fill available space
Grid.Auto = ({
	minWidth = "16rem",
	...props
}: Omit<GridProps, "cols"> & { minWidth?: string }) => (
	<Grid
		responsive={false}
		className={cn(
			`grid-cols-[repeat(auto-fit,minmax(min(${minWidth},100%),1fr))]`,
			props.className,
		)}
		{...props}
	/>
);

// Smart grid that adapts layout based on number of items and container size
// Inspired by "always great grid" - uses container queries for optimal layouts
Grid.Smart = ({
	minItemWidth = "250px",
	maxCols = 6,
	...props
}: Omit<GridProps, "cols"> & { minItemWidth?: string; maxCols?: number }) => (
	<Grid
		responsive={false}
		className={cn(
			// Base auto-fit grid that adapts to container
			`grid-cols-[repeat(auto-fit,minmax(min(${minItemWidth},100%),1fr))]`,
			// Ensure items stretch to fill space
			"auto-cols-fr",
			props.className,
		)}
		style={{
			gridTemplateColumns: `repeat(auto-fit, minmax(min(${minItemWidth}, 100%), 1fr))`,
			...props.style,
		}}
		{...props}
	/>
);

// Common 2-column layouts
Grid.Halves = (props: Omit<GridProps, "cols">) => (
	<Grid cols={2} gap={props.gap || 4} {...props} />
);

// Common 3-column layouts
Grid.Thirds = (props: Omit<GridProps, "cols">) => (
	<Grid cols={3} gap={props.gap || 4} {...props} />
);

// Common 4-column layouts
Grid.Quarters = (props: Omit<GridProps, "cols">) => (
	<Grid cols={4} gap={props.gap || 4} {...props} />
);

// Sidebar + main content layout
Grid.Sidebar = (props: Omit<GridProps, "cols"> & { sidebarWidth?: string }) => (
	<Grid
		cols={2}
		className={cn(
			props.sidebarWidth
				? `grid-cols-[${props.sidebarWidth}_1fr]`
				: "grid-cols-[16rem_1fr]",
			props.className,
		)}
		gap={0}
		{...props}
	/>
);

// Card grid with responsive columns
Grid.Cards = (props: Omit<GridProps, "cols">) => (
	<Grid
		className={cn(
			"grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4",
			props.className,
		)}
		gap={props.gap || 4}
		{...props}
	/>
);

// Form grid with label/input pairs
Grid.Form = (props: Omit<GridProps, "cols" | "align">) => (
	<Grid
		cols={1}
		gap={props.gap || 4}
		className={cn("max-w-2xl", props.className)}
		{...props}
	/>
);

// Gallery grid with square aspect ratio items
Grid.Gallery = (props: Omit<GridProps, "cols">) => (
	<Grid
		className={cn(
			"grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5",
			"*:aspect-square",
			props.className,
		)}
		gap={props.gap || 2}
		{...props}
	/>
);

// Masonry-like layout (with dense packing)
Grid.Masonry = (props: Omit<GridProps, "flow">) => (
	<Grid flow="row-dense" gap={props.gap || 4} {...props} />
);

// Dashboard grid with defined areas
Grid.Dashboard = (props: GridProps) => (
	<Grid
		className={cn(
			"grid-cols-1 md:grid-cols-4 lg:grid-cols-6",
			"grid-rows-[auto_1fr_auto]",
			"min-h-screen",
			props.className,
		)}
		gap={props.gap || 4}
		{...props}
	/>
);

// Feature grid for landing pages
Grid.Features = (props: Omit<GridProps, "cols">) => (
	<Grid
		className={cn(
			"grid-cols-1 md:grid-cols-2 lg:grid-cols-3",
			"text-center",
			props.className,
		)}
		gap={props.gap || 8}
		{...props}
	/>
);

// Adaptive grid that uses quantity queries to optimize layout
// Based on "always great grid" - automatically adjusts based on number of items
// This ensures items always fill the horizontal space optimally
Grid.Adaptive = (props: Omit<GridProps, "cols" | "responsive">) => (
	<Grid
		responsive={false}
		className={cn("grid-adaptive", props.className)}
		{...props}
	/>
);

// Balanced grid that automatically sizes items based on content
// Items wrap naturally and fill available space, with rows sizing to fit content height
// Perfect for dashboards where items have varying content sizes
// Automatically adapts to any screen size without explicit column/row values
Grid.Balanced = ({
	minItemWidth = "min(300px, 100%)",
	...props
}: Omit<GridProps, "cols" | "responsive"> & {
	minItemWidth?: string;
}) => (
	<Grid
		responsive={false}
		flow="row"
		className={cn(
			`grid-cols-[repeat(auto-fit,minmax(${minItemWidth},1fr))]`,
			props.className,
		)}
		style={{
			gridAutoRows: "min-content",
			gridTemplateColumns: `repeat(auto-fit, minmax(${minItemWidth}, 1fr))`,
			...props.style,
		}}
		{...props}
	/>
);

// Bento-style grid with automatic size assignment
// Creates a playful bento-box layout with varied item sizes
// Responsive and adaptive to screen size with dense packing
Grid.Bento = ({
	cols = 6,
	rows,
	gap = 4,
	pad,
	fullWidth = true,
	fullHeight = false,
	className,
	style,
	children,
	...props
}: Omit<GridProps, "responsive">) => {
	return (
		<motion.div
			className={cn(
				"grid w-full p-4",
				// Dense packing to fill gaps on larger screens only
				"lg:grid-flow-dense",
				// Responsive columns: mobile (2) -> tablet (4) -> desktop (6)
				cols === 12 && "grid-cols-2 md:grid-cols-6 lg:grid-cols-12",
				cols === 6 && "grid-cols-2 md:grid-cols-4 lg:grid-cols-6",
				cols === 4 && "grid-cols-2 md:grid-cols-4",
				cols === 3 && "grid-cols-2 md:grid-cols-3",
				cols === 2 && "grid-cols-2",
				cols === 1 && "grid-cols-1",
				gap && gapClasses[gap],
				pad && padClasses[pad],
				fullHeight && "h-full",
				className,
			)}
			style={{
				gridAutoRows: "minmax(150px, auto)",
				...style,
			}}
			{...props}
		>
			{children}
		</motion.div>
	);
};
