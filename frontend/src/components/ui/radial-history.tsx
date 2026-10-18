import type { ReactElement } from "react";
import { cn } from "#/lib/utils";

export interface RadialHistoryProps {
	/* Number of sectors (e.g. grid regions). */
	sectors: number;
	/* Rows oldest first; each row holds one value in [0, 1] per sector (undefined = no reading). */
	rows: Array<Array<number | undefined>>;
	/* Sectors to outline (e.g. the regions in the current token). */
	highlight?: number[];
	size?: number;
	labels?: (sector: number) => string;
	className?: string;
}

export const sectorHue = (sector: number, sectors: number) =>
	`hsl(${Math.round((sector * 360) / Math.max(sectors, 1))} 85% 60%)`;

const polar = (c: number, r: number, a: number) => [
	c + r * Math.cos(a),
	c + r * Math.sin(a),
];

/*
RadialHistory is a radar of rings: one sector per region, the outer ring is
the latest reading and each ring inward is older, so a region that keeps
lighting up draws a bright spoke and a flash draws a single bright cell.
*/
export const RadialHistory = ({
	sectors,
	rows,
	highlight = [],
	size = 220,
	labels,
	className,
}: RadialHistoryProps) => {
	const c = size / 2;
	const outer = size / 2 - 16;
	const inner = outer * 0.22;
	const n = rows.length;
	const step = n ? (outer - inner) / n : 0;
	const wedge = (2 * Math.PI) / sectors;

	const cell = (s: number, r0: number, r1: number) => {
		const a0 = s * wedge - Math.PI / 2 + 0.02;
		const a1 = (s + 1) * wedge - Math.PI / 2 - 0.02;
		const [x0, y0] = polar(c, r1, a0);
		const [x1, y1] = polar(c, r1, a1);
		const [x2, y2] = polar(c, r0, a1);
		const [x3, y3] = polar(c, r0, a0);
		return `M${x0},${y0} A${r1},${r1} 0 0 1 ${x1},${y1} L${x2},${y2} A${r0},${r0} 0 0 0 ${x3},${y3} Z`;
	};

	const sectorIds = Array.from({ length: sectors }, (_, sector) => sector);
	const backgrounds = sectorIds.map((sector) => (
		<path
			key={sector}
			d={cell(sector, inner, outer)}
			fill="var(--sunken)"
			stroke="var(--line)"
			strokeWidth={0.5}
		/>
	));
	const cells: ReactElement[] = [];
	let ring = 0;

	for (const row of rows) {
		let sector = 0;

		for (const value of row) {
			if (value !== undefined && value > 0.01) {
				cells.push(
					<path
						key={`${ring}:${sector}`}
						d={cell(sector, inner + ring * step, inner + (ring + 1) * step)}
						fill="var(--info)"
						opacity={Math.min(1, value) * (0.25 + (0.75 * (ring + 1)) / n)}
					/>,
				);
			}

			sector += 1;
		}

		ring += 1;
	}
	const marks = labels
		? sectorIds.map((sector) => {
				const [x, y] = polar(
					c,
					outer + 9,
					(sector + 0.5) * wedge - Math.PI / 2,
				);
				return (
					<text
						key={sector}
						x={x}
						y={y}
						fontSize={8}
						textAnchor="middle"
						dominantBaseline="middle"
						fill="var(--f3)"
						className="font-mono"
					>
						{labels(sector)}
					</text>
				);
			})
		: [];

	return (
		<svg
			width={size}
			height={size}
			viewBox={`0 0 ${size} ${size}`}
			className={cn("shrink-0", className)}
		>
			<title>Ring</title>
			{backgrounds}
			{cells}
			{highlight.map((sector) => (
				<path
					key={sector}
					d={cell(sector, inner, outer + 4)}
					fill="none"
					stroke="var(--f1)"
					strokeWidth={1.5}
				/>
			))}
			{marks}
			{n === 0 && (
				<text
					x={c}
					y={c}
					fontSize={10}
					textAnchor="middle"
					fill="var(--f4)"
					className="font-mono"
				>
					waiting
				</text>
			)}
		</svg>
	);
};
