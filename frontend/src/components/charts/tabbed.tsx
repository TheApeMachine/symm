import { TabsTab } from "node_modules/@base-ui/react/esm/tabs/tab/TabsTab";
import { Tabs } from "#/components/ui/tabs";

interface TabbedChartProps {
	tabs: {
		label: string;
		icon: React.ReactNode;
		component: React.ReactNode;
	}[];
}

export const TabbedChart = ({ tabs }: TabbedChartProps) => {
	return (
		<Tabs className="w-full h-full items-center p-0 gap-0" defaultValue="tab-1">
			<div className="border-b w-full">
				<Tabs.List variant="underline" className="gap-2">
					{tabs.map((tab) => (
						<TabsTab
							className="h-auto! flex-col gap-1.5 py-[calc(--spacing(2)-1px)]"
							value={tab.label}
							key={tab.label}
						>
							{tab.icon}
						</TabsTab>
					))}
				</Tabs.List>
			</div>
			{tabs.map((tab) => (
				<Tabs.Panel value={tab.label} key={tab.label} className="w-full h-full">
					{tab.component}
				</Tabs.Panel>
			))}
		</Tabs>
	);
};
