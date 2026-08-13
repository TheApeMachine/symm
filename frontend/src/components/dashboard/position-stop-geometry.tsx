import { Typography } from "#/components/ui/typography";
import { Flex } from "@/components/ui/flex";

/*
Floor and Peak bound the live stop interval. Before protection locks, Floor is
the forecast loss boundary. Afterward it is the locked floor that ratchets with
new peaks, so this domain follows the regulator's phase without reconstructing
that phase in the browser.
*/
const stopDomain = ["holding.stoploss.floor", "holding.stoploss.peak"].join(
	",",
);

const stopMarkers = [
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
PositionStopGeometry maps the current mark onto the regulator's active stop
interval. Profit-lock boundaries remain named underneath, but do not compress
the distance between the floor and peak that determines stop proximity.
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
				data-paint-class="armed:text-(--up) triggered:text-(--down) error:text-(--down)"
			/>
		</Flex.Row>
	</>
);
