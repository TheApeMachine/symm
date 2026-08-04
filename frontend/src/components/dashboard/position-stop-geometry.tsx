import { Typography } from "#/components/ui/typography";
import { Flex } from "@/components/ui/flex";

const stopDomain = [
	"holding.entry_price",
	"holding.mark",
	"holding.stoploss.hard_floor",
	"holding.stoploss.break_even_line",
	"holding.stoploss.profit_failsafe",
	"holding.stoploss.profit_line",
	"holding.stoploss.arm_line",
	"holding.stoploss.profit_floor",
	"holding.stoploss.trail_floor",
	"holding.stoploss.floor",
	"holding.stoploss.peak",
].join(",");

const stopMarkers = [
	{
		path: "holding.stoploss.hard_floor",
		title: "hard loss boundary",
		className: "h-3 w-px bg-(--down)",
	},
	{
		path: "holding.stoploss.break_even_line",
		title: "round-trip break-even",
		className: "h-2.5 w-px bg-(--f2)",
	},
	{
		path: "holding.stoploss.profit_line",
		title: "minimum profitable exit",
		className: "h-2.5 w-px bg-(--info)",
	},
	{
		path: "holding.stoploss.profit_failsafe",
		title: "immediate protected-profit failsafe",
		className: "h-2 w-px bg-(--warn)",
	},
	{
		path: "holding.stoploss.arm_line",
		title: "profit protection arms here",
		className: "h-3 w-px bg-(--warn)",
	},
	{
		path: "holding.stoploss.profit_floor",
		title: "profit locked when protection arms",
		className: "h-2.5 w-px bg-(--up)",
	},
	{
		path: "holding.stoploss.trail_floor",
		title: "peak-following giveback floor",
		className: "h-2.5 w-px bg-(--info)",
	},
	{
		path: "holding.stoploss.floor",
		title: "active exit floor",
		className: "h-3.5 w-[2px] rounded-full bg-(--acc)",
	},
	{
		path: "holding.entry_price",
		title: "entry",
		className: "h-2 w-px bg-(--f4)",
	},
	{
		path: "holding.stoploss.peak",
		title: "reachable peak",
		className: "h-3 w-px bg-(--up)",
	},
	{
		path: "holding.mark",
		title: "executable mark",
		className: "h-2 w-2 rounded-full border border-(--surface) bg-(--f1)",
	},
] as const;

const stopLevels = [
	["break-even", "holding.stoploss.break_even_line", "text-(--f2)"],
	["profit", "holding.stoploss.profit_line", "text-(--info)"],
	["arm", "holding.stoploss.arm_line", "text-(--warn)"],
	["lock", "holding.stoploss.profit_floor", "text-(--f3)"],
	["trail", "holding.stoploss.trail_floor", "text-(--f3)"],
	["failsafe", "holding.stoploss.profit_failsafe", "text-(--f3)"],
] as const;

/*
PositionStopGeometry maps every regulator boundary onto one price axis, then
names the contract values underneath. Values come directly from Stoploss; only
their position within the shared domain is derived in the browser.
*/
export const PositionStopGeometry = () => (
	<>
		<div className="relative mt-2 h-1 overflow-visible rounded-full bg-[linear-gradient(90deg,color-mix(in_srgb,var(--down)_12%,transparent),color-mix(in_srgb,var(--f4)_18%,transparent)_42%,color-mix(in_srgb,var(--up)_12%,transparent))]">
			{stopMarkers.map((marker) => (
				<div
					key={marker.path}
					data-set={marker.path}
					data-set-domain={stopDomain}
					data-set-scale="domain-percent"
					data-target="style.left"
					title={marker.title}
					className={`pointer-events-none absolute top-1/2 -translate-x-1/2 -translate-y-1/2 ${marker.className}`}
				/>
			))}
		</div>

		<Flex.Row className="mt-1.25 items-center justify-between gap-2 text-[8.5px]">
			<Typography.Span className="text-(--down)">
				hard{" "}
				<span
					data-paint="holding.stoploss.hard_floor"
					data-paint-format=".6f"
				/>
			</Typography.Span>
			<Typography.Span className="text-(--acc)">
				active{" "}
				<span data-paint="holding.stoploss.floor" data-paint-format=".6f" />
			</Typography.Span>
			<Typography.Span className="text-(--up)">
				peak <span data-paint="holding.stoploss.peak" data-paint-format=".6f" />
			</Typography.Span>
		</Flex.Row>

		<div className="mt-1 grid grid-cols-3 gap-x-2 gap-y-0.5 border-(--line) border-t pt-1 text-[8px] text-(--f4)">
			{stopLevels.map(([label, path, className]) => (
				<span key={path}>
					{label}{" "}
					<b
						className={`font-normal ${className}`}
						data-paint={path}
						data-paint-format=".6f"
					/>
				</span>
			))}
		</div>

		<Flex.Row className="mt-1 items-center justify-between gap-2 text-[8px] text-(--f4)">
			<span>
				noise{" "}
				<b
					className="font-normal text-(--f3)"
					data-paint="holding.stoploss.plan.noise_band"
					data-paint-format=".6f"
				/>
			</span>
			<span>
				confirm{" "}
				<b
					className="font-normal text-(--f3)"
					data-paint="holding.stoploss.plan.confirm_marks"
				/>{" "}
				marks
			</span>
			<span>
				basis{" "}
				<b
					className="font-normal"
					data-paint="holding.stoploss.basis_confirmed"
					data-paint-class="true:text-(--up) false:text-(--warn)"
				/>
			</span>
			<span>
				events{" "}
				<b
					className="font-normal text-(--f3)"
					data-paint="holding.stoploss.transitions.length"
				/>
			</span>
		</Flex.Row>

		<Typography.Span
			data-paint="holding.stoploss.trigger_reason"
			data-paint-class="hard_risk:text-(--down) protected_giveback:text-(--warn) profit_failsafe:text-(--warn)"
			className="mt-0.5 text-right text-[8px] uppercase"
		/>
	</>
);
