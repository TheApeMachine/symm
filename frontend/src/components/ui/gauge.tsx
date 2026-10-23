import { type ReactNode, useId } from "react";
import { cn } from "#/lib/utils";

export type GaugeTone = "up" | "down" | "warn" | "acc" | "info" | "muted";

export const toneColor = (tone: GaugeTone): string =>
	tone === "muted" ? "var(--f4)" : `var(--${tone})`;

export interface GaugeProps {
	/* Fill in [0, 1]; undefined draws the empty (waiting) track. */
	value?: number;
	tone?: GaugeTone;
	size?: number;
	/* Angle swept by the track, in degrees. */
	sweep?: number;
	label?: ReactNode;
	caption?: ReactNode;
	/* Optional tick positions in [0, 1], e.g. a danger line. */
	ticks?: number[];
	className?: string;
}

const polar = (cx: number, cy: number, r: number, deg: number) => {
	const rad = ((deg - 90) * Math.PI) / 180;
	return [cx + r * Math.cos(rad), cy + r * Math.sin(rad)] as const;
};

const arc = (cx: number, cy: number, r: number, from: number, to: number) => {
	const [x1, y1] = polar(cx, cy, r, from);
	const [x2, y2] = polar(cx, cy, r, to);
	const large = to - from > 180 ? 1 : 0;
	return `M ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2}`;
};

/*
Gauge is an arc dial: an empty track, a filled sweep for the value, and the
label in the middle. With no value it shows only the track.

Fill uses stroke-dasharray on the fixed track path with square caps only —
no round caps and no separate tip marker.
*/
export const Gauge = ({
	value,
	tone = "acc",
	size = 96,
	sweep = 240,
	label,
	caption,
	ticks = [],
	className,
}: GaugeProps) => {
	const clipId = useId();
	const c = size / 2;
	const stroke = 7;
	const r = size / 2 - stroke / 2 - 2;
	const start = -sweep / 2;
	const v = value === undefined ? undefined : Math.max(0, Math.min(1, value));
	const trackPath = arc(c, c, r, start, start + sweep);
	const fillColor = toneColor(tone);

	return (
		<div
			className={cn("relative inline-flex flex-col items-center", className)}
		>
			<svg
				width={size}
				height={size}
				viewBox={`0 0 ${size} ${size}`}
				role="img"
				className="overflow-hidden"
			>
				<title>Gauge</title>
				<defs>
					<clipPath id={clipId}>
						<circle cx={c} cy={c} r={size / 2 - 1} />
					</clipPath>
				</defs>
				<g clipPath={`url(#${clipId})`}>
					<path
						d={trackPath}
						stroke="var(--line)"
						strokeWidth={stroke}
						fill="none"
						strokeLinecap="butt"
					/>
					{v !== undefined && v > 0.001 && (
						<path
							d={trackPath}
							stroke={fillColor}
							strokeWidth={stroke}
							fill="none"
							strokeLinecap="butt"
							pathLength={1}
							strokeDasharray={`${v} ${1 - v}`}
							style={{
								transition: "stroke-dasharray 180ms ease-out",
							}}
						/>
					)}
					{ticks.map((t) => {
						const [x1, y1] = polar(c, c, r - 5, start + sweep * t);
						const [x2, y2] = polar(c, c, r + 5, start + sweep * t);
						return (
							<line
								key={t}
								x1={x1}
								y1={y1}
								x2={x2}
								y2={y2}
								stroke="var(--f3)"
								strokeWidth={1.5}
							/>
						);
					})}
				</g>
			</svg>
			<div className="absolute inset-0 flex items-center justify-center text-center font-mono text-[12px] text-(--f1) font-bold">
				{v === undefined ? (
					<span className="text-(--f4) font-normal">waiting</span>
				) : (
					label
				)}
			</div>
			{caption && (
				<div className="-mt-3 text-[10px] text-(--f3) text-center leading-tight">
					{caption}
				</div>
			)}
		</div>
	);
};
