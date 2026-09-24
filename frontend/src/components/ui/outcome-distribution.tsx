import { area, curveBasis, line } from "d3-shape";
import { type ComponentProps, useEffect, useRef, useState } from "react";
import { cn } from "#/lib/utils";
import { Flex } from "./flex";

export interface OutcomeBin {
	id: string;
	lower: number;
	upper: number;
	count: number;
}
export type OutcomeDistributionProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	bins?: OutcomeBin[] | null;
	/** Mean and standard deviation of the outcomes, in unit. */
	mean?: number;
	deviation?: number;
	unit?: string;
	title?: string;
	note?: string;
};

const finite = (value: number | string | null | undefined) => {
	if (value === undefined || value === null || value === "") return undefined;
	const parsed = typeof value === "number" ? value : Number(value);
	return Number.isFinite(parsed) ? parsed : undefined;
};

const density = (x: number, mean: number, deviation: number) =>
	Math.exp(-0.5 * ((x - mean) / deviation) ** 2) /
	(deviation * Math.sqrt(2 * Math.PI));

/*
OutcomeDistribution draws completed outcomes: the counts the graph binned, and
the normal curve the graph's mean and deviation describe, against breakeven.
*/
export const OutcomeDistribution = ({
	bins,
	mean,
	deviation,
	unit = "",
	title = "Edge Distribution",
	note = "Normal fit to the mean and deviation of completed outcomes.",
	className,
	...props
}: OutcomeDistributionProps) => {
	const ref = useRef<HTMLDivElement>(null);
	const [size, setSize] = useState({ width: 400, height: 180 });

	useEffect(() => {
		const target = ref.current;
		if (!target || typeof ResizeObserver === "undefined") return;
		const observer = new ResizeObserver((entries) => {
			const box = entries[0]?.contentRect;
			if (box && box.width > 0 && box.height > 0)
				setSize({ width: box.width, height: box.height });
		});
		observer.observe(target);
		return () => observer.disconnect();
	}, []);

	for (const bin of bins ?? []) {
		if (!(bin.upper > bin.lower) || bin.count < 0)
			throw new Error(`Invalid outcome bin ${bin.id}`);
	}

	const mu = finite(mean);
	const sigma = finite(deviation);
	const fitted = mu !== undefined && sigma !== undefined && sigma > 0;
	const empty = !bins?.length && !fitted;

	let lower = 0;
	let upper = 0;
	for (const bin of bins ?? []) {
		lower = Math.min(lower, bin.lower);
		upper = Math.max(upper, bin.upper);
	}
	if (fitted) {
		lower = Math.min(lower, mu - 3 * sigma);
		upper = Math.max(upper, mu + 3 * sigma);
	}
	const span = upper - lower || 1;

	const { width, height } = size;
	const base = height - 25;
	const x = (value: number) => 20 + ((value - lower) / span) * (width - 40);

	const curve: { x: number; y: number }[] = [];
	if (fitted) {
		for (let step = 0; step <= 120; step++) {
			const at = lower + (span * step) / 120;
			curve.push({ x: at, y: density(at, mu, sigma) });
		}
	}
	const top = Math.max(...curve.map((point) => point.y), 0);
	const y = (value: number) =>
		top === 0 ? base : base - (value / top) * (base - 24);
	const peak = Math.max(...(bins ?? []).map((bin) => bin.count), 0);

	return (
		<Flex.Column
			className={cn(
				"min-h-0 rounded border border-(--line) bg-(--surface) font-mono text-[11px] text-(--f3)",
				className,
			)}
			{...props}
		>
			<Flex.Row className="h-8 shrink-0 items-center gap-4 border-(--line) border-b bg-(--sunken) px-4 text-[10px] uppercase tracking-widest">
				<span className="text-(--f4)">Learn</span>
				<span className="text-(--acc)">{title}</span>
			</Flex.Row>
			<div ref={ref} className="relative min-h-32 flex-1">
				{empty && (
					<span className="absolute inset-0 flex items-center justify-center text-(--f4)">
						No outcome distribution
					</span>
				)}
				{!empty && (
					<svg
						width={width}
						height={height}
						role="img"
						aria-label="Outcome distribution"
						className="absolute inset-0"
					>
						<title>Recorded outcome counts</title>
						<defs>
							<linearGradient id="outcomeFill" x1="0" y1="0" x2="0" y2="1">
								<stop offset="0%" stopColor="var(--info)" stopOpacity="0.3" />
								<stop offset="100%" stopColor="var(--info)" stopOpacity="0" />
							</linearGradient>
						</defs>
						{bins?.map((bin) => (
							<rect
								key={bin.id}
								x={x(bin.lower)}
								y={peak === 0 ? base : base - (bin.count / peak) * (base - 24)}
								width={x(bin.upper) - x(bin.lower)}
								height={peak === 0 ? 0 : (bin.count / peak) * (base - 24)}
								fill="var(--info)"
								opacity={fitted ? 0.18 : 0.6}
								stroke="var(--sunken)"
							>
								<title>{`${bin.lower} to ${bin.upper} ${unit}: ${bin.count}`}</title>
							</rect>
						))}
						<line
							x1={20}
							x2={width - 20}
							y1={base}
							y2={base}
							stroke="var(--line)"
						/>
						<line
							x1={x(0)}
							x2={x(0)}
							y1={20}
							y2={base}
							stroke="var(--f4)"
							strokeDasharray="2 2"
						/>
						<text
							x={x(0)}
							y={15}
							fill="var(--f4)"
							fontSize="9"
							textAnchor="middle"
						>
							0.0 {unit}
						</text>
						{fitted && (
							<>
								<path
									d={
										area<{ x: number; y: number }>()
											.x((point) => x(point.x))
											.y0(base)
											.y1((point) => y(point.y))
											.curve(curveBasis)(curve) ?? undefined
									}
									fill="url(#outcomeFill)"
								/>
								<path
									d={
										line<{ x: number; y: number }>()
											.x((point) => x(point.x))
											.y((point) => y(point.y))
											.curve(curveBasis)(curve) ?? undefined
									}
									fill="none"
									stroke="var(--info)"
									strokeWidth="1.5"
								/>
								<line
									x1={x(mu)}
									x2={x(mu)}
									y1={20}
									y2={base}
									stroke={mu > 0 ? "var(--up)" : "var(--down)"}
								/>
								<text
									x={x(mu)}
									y={28}
									fill={mu > 0 ? "var(--up)" : "var(--down)"}
									fontSize="9"
									textAnchor="middle"
								>
									μ {mu.toFixed(1)}
								</text>
							</>
						)}
						<text x={20} y={height - 6} fill="var(--f4)" fontSize="9">
							{lower.toFixed(1)} {unit}
						</text>
						<text
							x={width - 20}
							y={height - 6}
							fill="var(--f4)"
							fontSize="9"
							textAnchor="end"
						>
							{upper.toFixed(1)} {unit}
						</text>
					</svg>
				)}
			</div>
			<div className="border-(--line) border-t p-3 text-[10px] text-(--f3)">
				{note}
			</div>
		</Flex.Column>
	);
};
