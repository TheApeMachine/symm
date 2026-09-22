import type { ComponentProps } from "react";
import { Flex } from "./flex";
import { Typography } from "./typography";

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
	unit?: string;
};

/* The graph supplies bin boundaries and counts; the view only scales geometry. */
export const OutcomeDistribution = ({
	bins,
	unit = "",
	...props
}: OutcomeDistributionProps) => {
	let minimum = bins?.[0]?.lower;
	let maximum = bins?.[0]?.upper;
	let peak = 0;
	for (const bin of bins ?? []) {
		if (!(bin.upper > bin.lower) || bin.count < 0)
			throw new Error(`Invalid outcome bin ${bin.id}`);
		minimum = Math.min(minimum as number, bin.lower);
		maximum = Math.max(maximum as number, bin.upper);
		peak = Math.max(peak, bin.count);
	}
	const span = (maximum as number) - (minimum as number);
	return (
		<Flex.Column {...props}>
			<Typography.Label>Outcome distribution</Typography.Label>
			{!bins?.length && (
				<Typography.Mono>No outcome distribution</Typography.Mono>
			)}
			{!!bins?.length && (
				<svg
					viewBox="0 0 400 180"
					role="img"
					aria-label="Outcome distribution"
					className="w-full min-h-32"
				>
					<title>Recorded outcome counts</title>
					{bins.map((bin) => (
						<rect
							key={bin.id}
							x={20 + ((bin.lower - (minimum as number)) / span) * 360}
							y={peak === 0 ? 150 : 150 - (bin.count / peak) * 130}
							width={((bin.upper - bin.lower) / span) * 360}
							height={peak === 0 ? 0 : (bin.count / peak) * 130}
							fill="var(--info)"
							stroke="var(--sunken)"
						>
							<title>{`${bin.lower} to ${bin.upper} ${unit}: ${bin.count}`}</title>
						</rect>
					))}
					<text x="20" y="172" fill="var(--f3)" fontSize="10">
						{minimum} {unit}
					</text>
					<text x="380" y="172" textAnchor="end" fill="var(--f3)" fontSize="10">
						{maximum} {unit}
					</text>
				</svg>
			)}
		</Flex.Column>
	);
};
