import { Typography } from "#/components/ui/typography";
import { Flex } from "@/components/ui/flex";

/*
The regulator publishes seven prices and one flag. Everything drawn here is one
of them: the axis is their shared domain, and each marker is a price placed on
it. Nothing is reconstructed in the browser, so a boundary that moves on the
engine moves here on the same frame.
*/
const stopDomain = [
	"holding.entry_price",
	"holding.mark",
	"holding.stoploss.floor",
	"holding.stoploss.profit_line",
	"holding.stoploss.arm_at",
	"holding.stoploss.lock_floor",
	"holding.stoploss.peak",
].join(",");

const stopMarkers = [
	{
		path: "holding.stoploss.lock_floor",
		title: "floor once profit protection arms",
		className: "h-2.5 w-px bg-(--up)",
	},
	{
		path: "holding.stoploss.profit_line",
		title: "minimum profitable exit",
		className: "h-2.5 w-px bg-(--info)",
	},
	{
		path: "holding.stoploss.arm_at",
		title: "profit protection arms here",
		className: "h-3 w-px bg-(--warn)",
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
		title: "highest mark seen",
		className: "h-3 w-px bg-(--up)",
	},
	{
		path: "holding.mark",
		title: "executable mark",
		className: "h-2 w-2 rounded-full border border-(--surface) bg-(--f1)",
	},
] as const;

const stopLevels = [
	["profit", "holding.stoploss.profit_line", "text-(--info)"],
	["arm", "holding.stoploss.arm_at", "text-(--warn)"],
	["lock", "holding.stoploss.lock_floor", "text-(--up)"],
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
			<Typography.Span className="text-(--acc)">
				floor{" "}
				<span data-paint="holding.stoploss.floor" data-paint-format=".6f" />
			</Typography.Span>
			<Typography.Span className="text-(--f3)">
				mark <span data-paint="holding.stoploss.mark" data-paint-format=".6f" />
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

		{/*
			Locked is the one thing on the regulator that is not a price: it says
			whether the floor has already ratcheted past break-even, which is the
			difference between a lot that can still lose and one that cannot.
		*/}
		<Flex.Row className="mt-1 items-center justify-between gap-2 text-[8px] text-(--f4)">
			<span>
				locked{" "}
				<b
					className="font-normal"
					data-paint="holding.stoploss.locked"
					data-paint-class="true:text-(--up) false:text-(--warn)"
				/>
			</span>
			<span>
				threshold{" "}
				<b
					className="font-normal text-(--f3)"
					data-paint="holding.profit_threshold"
					data-paint-format=".6f"
				/>
			</span>
			<b
				className="font-normal uppercase"
				data-paint="holding.stoploss.status"
				data-paint-class="ARMED:text-(--up) TRIGGERED:text-(--down) ERROR:text-(--down)"
			/>
		</Flex.Row>
	</>
);
