import { area, curveMonotoneX, line } from "d3-shape";
import type { ComponentProps } from "react";
import { cn } from "#/lib/utils";
import { Flex } from "./flex";

export type KernelCardProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	name?: string;
	/** What the kernel measures, in a few words. */
	description?: string;
	/** The kernel's latest reading. */
	value?: number;
	/** Its recent readings, oldest first. */
	points?: number[];
};

/* compact writes a reading with four significant digits, the way a card has room for. */
export const compact = (value: number) =>
	Math.abs(value) >= 1e4 || (Math.abs(value) > 0 && Math.abs(value) < 1e-3)
		? value.toExponential(2)
		: value.toPrecision(4);

/*
KernelCard is one signal kernel at a glance: what it is, its latest reading and
how the reading has moved. Only what the graph delivered is drawn; a kernel
that has said nothing yet reads as waiting.
*/
export const KernelCard = ({
	name = "",
	description,
	value,
	points,
	className,
	...props
}: KernelCardProps) => {
	const width = 240;
	const height = 44;
	const series = points ?? [];
	let low = series[0] ?? 0;
	let high = series[0] ?? 0;
	for (const point of series) {
		low = Math.min(low, point);
		high = Math.max(high, point);
	}
	const x = (index: number) =>
		series.length > 1 ? (index / (series.length - 1)) * width : width / 2;
	const y = (point: number) =>
		high === low
			? height / 2
			: height - 3 - ((point - low) / (high - low)) * (height - 6);
	const indexed = series.map((point, index) => ({ index, point }));
	const stroke =
		series.length > 1
			? (line<{ index: number; point: number }>()
					.x((entry) => x(entry.index))
					.y((entry) => y(entry.point))
					.curve(curveMonotoneX)(indexed) ?? "")
			: "";
	const fill =
		series.length > 1
			? (area<{ index: number; point: number }>()
					.x((entry) => x(entry.index))
					.y0(height)
					.y1((entry) => y(entry.point))
					.curve(curveMonotoneX)(indexed) ?? "")
			: "";
	// A reading that was not a finite number arrives as null and is not a reading.
	const live = typeof value === "number";

	return (
		<Flex.Column
			className={cn(
				"gap-1.5 border-(--line) border-b px-3 py-2.5 font-mono",
				className,
			)}
			{...props}
		>
			<Flex.Row className="items-start justify-between gap-2">
				<span className="font-sans font-semibold text-[13px] text-(--f1) leading-tight">
					{name}
				</span>
				<span
					className={cn(
						"shrink-0 rounded-xs border px-1.5 py-px text-[9px] uppercase tracking-wide",
						live
							? "border-(--up)/40 text-(--up)"
							: "border-(--line2) text-(--f4)",
					)}
				>
					{live ? "reporting" : "waiting"}
				</span>
			</Flex.Row>
			{description && (
				<span className="truncate text-[10px] text-(--f4)">{description}</span>
			)}
			<svg
				viewBox={`0 0 ${width} ${height}`}
				preserveAspectRatio="none"
				role="img"
				aria-label={`${name} readings`}
				className="h-11 w-full"
			>
				<title>{`${name} readings`}</title>
				<path d={fill} fill="var(--acc)" fillOpacity={0.14} />
				<path
					d={stroke}
					fill="none"
					stroke="var(--acc)"
					strokeWidth={1.25}
					vectorEffect="non-scaling-stroke"
				/>
			</svg>
			<Flex.Row className="items-baseline justify-between text-[10px] text-(--f4)">
				<span>
					{series.length ? `${series.length} readings` : "no readings"}
				</span>
				<span
					className={cn("text-[12px]", live ? "text-(--f1)" : "text-(--f4)")}
				>
					{typeof value === "number" ? compact(value) : "—"}
				</span>
			</Flex.Row>
		</Flex.Column>
	);
};
