import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { Panel } from "@/components/ui/panel";

/*
CrossSectionPanel states the liquidity cross-section for the focused symbol.

The medians here are the liquidity kernel's own — it maintains a rolling median
of reported notional and executable touch depth, and reports where the current
reading sits against them. The panel used to compute its own medians across
whichever measurement rows happened to arrive in one frame, which is not a
market cross-section at all: the measurements key is a rolling delta, so that
aggregate changed shape every tick and agreed with nothing the engine believed.

The bar is the reading against its own median, so a symbol whose depth is
sitting at its typical level reads mid-scale rather than full.
*/
const STATS = [
	{
		label: "med notional",
		path: "metrics.reported_volume_notional_median.raw",
		align: "text-left",
	},
	{
		label: "med depth",
		path: "metrics.executable_touch_depth_median.raw",
		align: "text-center",
	},
	{
		label: "touch depth",
		path: "metrics.executable_touch_depth.raw",
		align: "text-right",
	},
] as const;

export const CrossSectionPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="measurements">
			{({ ref }) => (
				<Panel
					ref={ref}
					size="lg"
					data-scope="source,symbol"
					data-filter={`liquidity,${focusSymbol}`}
				>
					<div className="flex items-center justify-between">
						<span className="font-semibold text-(--f1) text-xs">
							Cross-section
						</span>
						{/*
							Scarcity is the kernel's own verdict on whether the book is
							thin, so the badge is driven by that reading rather than by a
							breadth threshold restated in the browser.
						*/}
						<span
							data-paint="metrics.scarcity_score.raw"
							data-paint-format=".3f"
							data-set="metrics.scarcity_score.raw"
							data-set-scale="sign-color"
							data-target="style.color"
							className="rounded-[3px] border border-(--line2) px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
						/>
					</div>
					<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
						liquidity axes ·{" "}
						<span data-paint="symbol" data-paint-empty="no focus" />
					</div>
					<div className="flex items-center justify-between">
						<span className="font-mono text-[11px] text-(--f2)">
							relative depth{" "}
							<span
								data-paint="metrics.relative_touch_depth.raw"
								data-paint-format=".3f"
								className="text-(--acc)"
							/>
						</span>
						<span
							data-paint="at"
							data-paint-format="time"
							className="font-mono text-[10px] text-(--f4)"
						/>
					</div>
					<div className="mt-2.5">
						<div
							className="flex items-center gap-2"
							role="progressbar"
							aria-valuemin={0}
							aria-valuemax={100}
						>
							<span className="w-13.5 shrink-0 font-mono text-[9px] text-(--f4)">
								Depth
							</span>
							<div className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line) [--meter-tone:var(--info)]">
								<div
									data-set="metrics.executable_touch_depth.raw"
									data-set-scale="domain-percent"
									data-set-domain="metrics.executable_touch_depth.raw,metrics.executable_touch_depth_median.raw"
									data-target="style.width"
									className="h-full bg-(--meter-tone)"
									style={{ width: "0%" }}
								/>
							</div>
							<span
								data-paint="metrics.executable_touch_depth.normalized"
								data-paint-format=".2f"
								className="w-7 shrink-0 text-right font-mono text-[9px] text-(--f2)"
							/>
						</div>
					</div>
					<div className="mt-3.25 flex justify-between gap-3">
						{STATS.map((stat) => (
							<div key={stat.path} className={`min-w-0 flex-1 ${stat.align}`}>
								<div
									data-paint={stat.path}
									data-paint-format=".0f"
									className="truncate font-mono text-lg text-(--f1) leading-none"
								/>
								<div className="mt-1 font-mono text-[9px] text-(--f4)">
									{stat.label}
								</div>
							</div>
						))}
					</div>
				</Panel>
			)}
		</Component>
	);
};
