import { FluidSurfaceChart } from "./symm/FluidSurfaceChart";
import { DecisionsPanel } from "./decisions";
import { TradesPanel } from "./trades";
import { SidebarSection } from "./sidebar-section";

export { SidebarSection } from "./sidebar-section";

export const DashboardSidebar = () => {
	return (
		<aside className="grid min-h-0 min-w-0 flex-3 grid-rows-[minmax(0,42%)_minmax(280px,58%)] overflow-hidden border-l border-(--dash-border) bg-(--dash-panel)">
			<div className="flex min-h-0 max-h-[42vh] overflow-hidden">
				<DecisionsPanel />
				<TradesPanel />
			</div>

			<div className="flex h-full min-h-[280px] min-w-0 flex-col overflow-hidden border-t border-(--dash-border) p-2">
				<FluidSurfaceChart className="h-full min-h-0 w-full" />
			</div>
		</aside>
	);
};
