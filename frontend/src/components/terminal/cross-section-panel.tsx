import { useSelector } from "@tanstack/react-store";
import { atoms, signals } from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";

const STATS = [
	{
		label: "best bid",
		name: "best_price:bid",
		format: "fixed:2",
		align: "text-left",
	},
	{
		label: "best ask",
		name: "best_price:ask",
		format: "fixed:2",
		align: "text-center",
	},
	{
		label: "rel spread",
		name: "relative_spread",
		format: "bp",
		align: "text-right",
	},
] as const;

export const CrossSectionPanel = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);

	return (
		<Panel size="lg">
			<Component store={signals.liquidity} symbol={focusSymbol}>
				<Flex.Row align="center" justify="between">
					<Typography.Label size="s" tone="f1" weight="semibold">
						Cross-section
					</Typography.Label>
					<Badge
						variant="info"
						size="xxs"
						label={
							<Typography.Span
								data-metric-raw="relative_spread"
								data-format="bp"
								className="text-[8px]"
							>
								—
							</Typography.Span>
						}
					/>
				</Flex.Row>

				<Typography.Paragraph className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					liquidity axes ·{" "}
					<Typography.Span data-metric="symbol">—</Typography.Span>
				</Typography.Paragraph>

				<Flex.Row align="center" justify="between">
					<Typography.Span className="font-mono text-[11px] text-(--f2)">
						relative spread{" "}
						<Typography.Span
							data-metric-raw="relative_spread"
							data-format="fixed:4"
							className="text-(--acc)"
						>
							—
						</Typography.Span>
					</Typography.Span>
					<Typography.Span
						data-metric="at"
						className="font-mono text-[10px] text-(--f4)"
					>
						—
					</Typography.Span>
				</Flex.Row>

				<div className="mt-2.5">
					<div
						className="flex items-center gap-2"
						role="progressbar"
						aria-valuemin={0}
						aria-valuemax={100}
					>
						<span className="w-13.5 shrink-0 font-mono text-[9px] text-(--f4)">
							Spread
						</span>
						<div className="h-1 flex-1 overflow-hidden rounded-[2px] bg-(--line)">
							<div
								data-mbar="relative_spread"
								className="h-full bg-(--info)"
								style={{ width: "0%" }}
							/>
						</div>
						<Typography.Span
							data-metric-raw="relative_spread"
							data-format="bp"
							className="w-12 shrink-0 text-right font-mono text-[9px] text-(--f2)"
						>
							—
						</Typography.Span>
					</div>
				</div>

				<div className="mt-3.25 flex justify-between gap-3">
					{STATS.map((stat) => (
						<div key={stat.name} className={`min-w-0 flex-1 ${stat.align}`}>
							<Typography.Mono
								size="lg"
								tone="f1"
								data-metric-raw={stat.name}
								data-format={stat.format}
								className="truncate leading-none"
							>
								—
							</Typography.Mono>
							<div className="mt-1 font-mono text-[9px] text-(--f4)">
								{stat.label}
							</div>
						</div>
					))}
				</div>
			</Component>
		</Panel>
	);
};
