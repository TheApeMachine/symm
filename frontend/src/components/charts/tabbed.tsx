import { useEffect, useState } from "react";
import { Tabs } from "#/components/ui/tabs";
import { cn } from "@/lib/utils";

interface TabbedChartProps {
	tabs: {
		label: string;
		icon: React.ReactNode;
		component: React.ReactNode;
	}[];
	className?: string;
	onActiveTabChange?: (label: string) => void;
}

export const TabbedChart = ({
	tabs,
	className,
	onActiveTabChange,
}: TabbedChartProps) => {
	const defaultTab = tabs[0]?.label ?? "";
	const [activeTab, setActiveTab] = useState(defaultTab);

	useEffect(() => {
		onActiveTabChange?.(activeTab);
	}, [activeTab, onActiveTabChange]);

	return (
		<Tabs
			className={cn("w-full h-full items-center p-0 gap-0", className)}
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
			<div className="relative min-h-0 w-full flex-1">
				{tabs.map((tab) => (
					<div
						key={tab.label}
						className={cn(
							"absolute inset-0 h-full w-full",
							activeTab !== tab.label && "invisible pointer-events-none",
						)}
						aria-hidden={activeTab !== tab.label}
					>
						{tab.component}
					</div>
				))}
			</div>
		</Tabs>
	);
};
