
import { useState, type ReactNode } from "react";

export type PanelTab = {
	key: string;
	label: string;
	content: ReactNode;
};

type PanelTabsProps = {
	id: string;
	tabs: PanelTab[];
	className?: string;
};

const loadActive = (id: string, fallback: string): string => {
	try {
		return localStorage.getItem(`panel-tab:${id}`) ?? fallback;
	} catch {
		return fallback;
	}
};

const saveActive = (id: string, key: string) => {
	try {
		localStorage.setItem(`panel-tab:${id}`, key);
	} catch {
		// ignore
	}
};

export const PanelTabs = ({ id, tabs, className = "" }: PanelTabsProps) => {
	const [active, setActive] = useState(() =>
		loadActive(id, tabs[0]?.key ?? ""),
	);

	const select = (key: string) => {
		setActive(key);
		saveActive(id, key);
	};

	const activeTab = tabs.find((t) => t.key === active) ?? tabs[0];

	return (
		<div className={`flex min-h-0 flex-col overflow-hidden ${className}`}>
			<div className="flex shrink-0 items-center gap-0.5 border-b border-(--dash-border) px-1 py-0.5">
				{tabs.map((tab) => (
					<button
						key={tab.key}
						type="button"
						onClick={() => select(tab.key)}
						className={`rounded px-2 py-0.5 text-[9px] font-medium uppercase tracking-wider transition-colors ${
							tab.key === activeTab?.key
								? "bg-(--dash-row-active) text-(--dash-accent)"
								: "text-(--dash-muted) hover:text-(--dash-text)"
						}`}
					>
						{tab.label}
					</button>
				))}
			</div>
			<div className="min-h-0 flex-1 overflow-hidden">
				{activeTab?.content}
			</div>
		</div>
	);
};

