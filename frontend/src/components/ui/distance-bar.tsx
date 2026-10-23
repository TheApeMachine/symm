import type { ReactNode } from "react";
import { cn } from "#/lib/utils";

export interface DistanceMarker {
	value: number;
	label: string;
	color?: string;
}

export interface DistanceBarProps {
	/* The reference point drawn in the middle (e.g. entry price). */
	origin?: number;
	/* The live point whose distance from origin is filled. */
	value?: number;
	/* Half-width of the bar in value units; defaults to |value - origin| * 1.5. */
	reach?: number;
	markers?: DistanceMarker[];
	leftLabel?: ReactNode;
	rightLabel?: ReactNode;
	className?: string;
}

/*
DistanceBar shows how far `value` sits from `origin` (centre tick). Fill is
green when value >= origin and red when below — price comparison only, not
P&L, fees, or exit economics.
*/
export const DistanceBar = ({
	origin,
	value,
	reach,
	markers = [],
	leftLabel,
	rightLabel,
	className,
}: DistanceBarProps) => {
	if (origin === undefined || value === undefined) {
		return (
			<div
				className={cn(
					"h-7 rounded border border-dashed border-(--line) text-[10px] text-(--f4) flex items-center justify-center",
					className,
				)}
			>
				waiting for a price
			</div>
		);
	}

	const span =
		reach && reach > 0
			? reach
			: Math.max(
					Math.abs(value - origin) * 1.5,
					Math.abs(origin) * 1e-4,
					1e-12,
				);
	const pos = (v: number) =>
		50 + Math.max(-50, Math.min(50, ((v - origin) / span) * 50));
	const p = pos(value);
	const up = value >= origin;

	return (
		<div className={cn("relative", className)}>
			<div className="relative h-7 rounded bg-(--sunken) border border-(--line) overflow-hidden">
				<div
					className="absolute top-0 bottom-0"
					style={{
						left: `${Math.min(50, p)}%`,
						width: `${Math.abs(p - 50)}%`,
						background: up
							? "linear-gradient(90deg, transparent, var(--up))"
							: "linear-gradient(270deg, transparent, var(--down))",
						opacity: 0.75,
						transition: "all 300ms",
					}}
				/>
				<div
					className="absolute top-0 bottom-0 w-0.5 bg-(--f1)"
					style={{ left: "50%" }}
				/>
				{markers.map((m) => (
					<div
						key={m.label}
						title={m.label}
						className="absolute top-0 bottom-0 w-0.5"
						style={{
							left: `${pos(m.value)}%`,
							background: m.color ?? "var(--info)",
						}}
					/>
				))}
				<div
					className="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-3 h-3 rounded-full border-2 border-(--bg)"
					style={{
						left: `${p}%`,
						background: up ? "var(--up)" : "var(--down)",
						transition: "left 300ms",
					}}
				/>
			</div>
			<div className="flex justify-between text-[9px] text-(--f4) mt-0.5">
				<span>{leftLabel}</span>
				<span>{rightLabel}</span>
			</div>
		</div>
	);
};
