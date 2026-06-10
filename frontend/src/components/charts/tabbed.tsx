import { useState } from "react";
import { Tabs } from "#/components/ui/tabs";

interface TabbedChartProps {
	tabs: {
		label: string;
		icon: React.ReactNode;
		component: React.ReactNode;
	}[];
}

export const TabbedChart = ({ tabs }: TabbedChartProps) => {
	const defaultTab = tabs[0]?.label ?? "";
	const [activeTab, setActiveTab] = useState(defaultTab);

	return (
		<Tabs
			className="w-full h-full items-center p-0 gap-0"
			value={activeTab}
			onValueChange={setActiveTab}
		>
			<div className="border-b w-full">
				<Tabs.List variant="underline" className="gap-2">
					{tabs.map((tab) => (
						<Tabs.Tab
							className="h-auto! flex-col gap-1.5 py-[calc(--spacing(2)-1px)]"
							value={tab.label}
							key={tab.label}
						>
							{tab.icon}
						</Tabs.Tab>
					))}
				</Tabs.List>
			</div>
			{tabs.map((tab) => (
				<Tabs.Panel value={tab.label} key={tab.label} className="w-full h-full">
					<div
						className="h-full w-full"
						hidden={activeTab !== tab.label}
						aria-hidden={activeTab !== tab.label}
					>
						{tab.component}
					</div>
				</Tabs.Panel>
			))}
		</Tabs>
	);
};
