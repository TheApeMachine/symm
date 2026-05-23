import { FluidSurfaceChart } from "./symm/FluidSurfaceChart";
import { DecisionsPanel } from "./desicions";
import { TradesPanel } from "./trades";

export const DashboardSidebar = () => {
	return (
		<aside className="grid min-h-0 min-w-0 flex-3 grid-rows-[minmax(0,38%)_minmax(240px,1fr)] overflow-hidden border-l border-(--dash-border) bg-(--dash-panel)">
			<div className="flex min-h-0 overflow-hidden">
				<DecisionsPanel />
				<TradesPanel />
			</div>

			<div className="flex min-h-0 overflow-hidden border-t border-(--dash-border) p-2">
				<FluidSurfaceChart className="min-h-0 h-full w-full" />
			</div>
		</aside>
	);
};

export const SidebarSection = ({
	title,
	children,
	className = "",
	fill = false,
}: {
	title: string;
	children: React.ReactNode;
	className?: string;
	fill?: boolean;
}) => {
	return (
		<section
			className={`${fill ? "flex min-h-0 flex-1 flex-col" : ""} ${className}`}
		>
			<h2 className="px-3 py-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-(--dash-muted)">
				{title}
			</h2>
			<div className={fill ? "min-h-0 flex-1 overflow-auto" : ""}>
				{children}
			</div>
		</section>
	);
};
