import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Panel } from "@/components/ui/panel";
import { measurementsStore, useSubscribe } from "#/providers/ws-stores";

const fmt = (value: unknown, digits: number): string =>
	typeof value === "number" ? value.toFixed(digits) : "—";

const metric = (
	row: { metrics?: Record<string, { raw: number; normalized?: number | null }> } | undefined,
	name: string,
): number | undefined => row?.metrics?.[name]?.raw;

const STATS = [
	{ label: "med notional", name: "reported_volume_notional_median", align: "text-left" },
	{ label: "med depth", name: "executable_touch_depth_median", align: "text-center" },
	{ label: "touch depth", name: "executable_touch_depth", align: "text-right" },
] as const;

export const CrossSectionPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	const root = useSubscribe(measurementsStore, (state) => {
		const row = state.measurements[`liquidity\u0000${focusSymbol}`]?.latest();

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("scarcity", fmt(metric(row, "scarcity_score"), 3));
		set("symbol", focusSymbol.length === 0 ? "no focus" : focusSymbol);
		set("rel", fmt(metric(row, "relative_touch_depth"), 3));
		set("at", row?.at === undefined ? "—" : new Date(row.at).toISOString().slice(11, 19));
		set("norm", fmt(row?.metrics?.executable_touch_depth?.normalized, 2));

		for (const stat of STATS) {
			set(stat.name, fmt(metric(row, stat.name), 0));
		}

		// depth bar
		const depth = metric(row, "executable_touch_depth");
		const median = metric(row, "executable_touch_depth_median");
		const bar = root.current?.querySelector<HTMLElement>("[data-depth-bar]");

		if (bar instanceof HTMLElement && depth !== undefined && median !== undefined && median > 0) {
			bar.style.width = `${Math.min(100, Math.max(0, (depth / median) * 100)).toFixed(3)}%`;
		}
	}, [focusSymbol]);

	return (
		<Panel ref={root} size="lg">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-(--f1) text-xs">Cross-section</span>
				<span data-f="scarcity" className="rounded-[3px] border border-(--line2) px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide">—</span>
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				liquidity axes · <span data-f="symbol" />
			</div>
			<div className="flex items-center justify-between">
				<span className="font-mono text-[11px] text-(--f2)">
					relative depth <span data-f="rel" className="text-(--acc)" />
				</span>
				<span data-f="at" className="font-mono text-[10px] text-(--f4)" />
			</div>
			<div className="mt-2.5">
				<div className="flex items-center gap-2" role="progressbar" aria-valuemin={0} aria-valuemax={100}>
					<span className="w-13.5 shrink-0 font-mono text-[9px] text-(--f4)">Depth</span>
					<div className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line) [--meter-tone:var(--info)]">
						<div data-depth-bar className="h-full bg-(--meter-tone)" style={{ width: "0%" }} />
					</div>
					<span data-f="norm" className="w-7 shrink-0 text-right font-mono text-[9px] text-(--f2)" />
				</div>
			</div>
			<div className="mt-3.25 flex justify-between gap-3">
				{STATS.map((stat) => (
					<div key={stat.name} className={`min-w-0 flex-1 ${stat.align}`}>
						<div data-f={stat.name} className="truncate font-mono text-lg text-(--f1) leading-none">—</div>
						<div className="mt-1 font-mono text-[9px] text-(--f4)">{stat.label}</div>
					</div>
				))}
			</div>
		</Panel>
	);
};
