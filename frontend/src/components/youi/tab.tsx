import {
	Children,
	type CSSProperties,
	isValidElement,
	type ReactElement,
	type ReactNode,
	useId,
	useState,
} from "react";
import { cn } from "#/lib/utils";

type TabLeaf = {
	kind: "tab";
	value: string;
	label: string;
	srLabel?: string;
};

type TabGroup = {
	kind: "group";
	title: string;
	tabs: TabLeaf[];
};

type Segment = TabLeaf | TabGroup;

type TabProps = {
	value: string;
	label: string;
	srLabel?: string;
};

type GroupProps = {
	title: string;
	children: ReactNode;
};

type TabsRootProps = {
	name?: string;
	value?: string;
	defaultValue?: string;
	onValueChange?: (value: string) => void;
	debug?: boolean;
	className?: string;
	children: ReactNode;
};

const parseTabLeaf = (child: ReactElement<TabProps>): TabLeaf => ({
	kind: "tab",
	value: child.props.value,
	label: child.props.label,
	srLabel: child.props.srLabel,
});

const parseSegments = (children: ReactNode): Segment[] => {
	const segments: Segment[] = [];

	Children.forEach(children, (child) => {
		if (!isValidElement(child)) {
			return;
		}

		if (child.type === Tab) {
			segments.push(parseTabLeaf(child as ReactElement<TabProps>));
			return;
		}

		if (child.type === Group) {
			const groupTabs: TabLeaf[] = [];

			Children.forEach((child.props as GroupProps).children, (groupChild) => {
				if (!isValidElement(groupChild) || groupChild.type !== Tab) {
					return;
				}

				groupTabs.push(parseTabLeaf(groupChild as ReactElement<TabProps>));
			});

			if (groupTabs.length === 0) {
				return;
			}

			segments.push({
				kind: "group",
				title: (child.props as GroupProps).title,
				tabs: groupTabs,
			});
		}
	});

	return segments;
};

const collectValues = (segments: Segment[]): string[] =>
	segments.flatMap((segment) =>
		segment.kind === "tab"
			? [segment.value]
			: segment.tabs.map((tab) => tab.value),
	);

const findActiveSegmentIndex = (
	segments: Segment[],
	activeValue: string | undefined,
): number => {
	if (!activeValue) {
		return 0;
	}

	const segmentIndex = segments.findIndex((segment) =>
		segment.kind === "tab"
			? segment.value === activeValue
			: segment.tabs.some((tab) => tab.value === activeValue),
	);

	return segmentIndex === -1 ? 0 : segmentIndex;
};

const findActiveGroupTabIndex = (
	segment: TabGroup,
	activeValue: string | undefined,
): number => {
	if (!activeValue) {
		return -1;
	}

	return segment.tabs.findIndex((tab) => tab.value === activeValue);
};

const Tab = (_props: TabProps) => {
	return null;
};
Tab.displayName = "Tabs.Tab";

const Group = (_props: GroupProps) => {
	return null;
};
Group.displayName = "Tabs.Group";

const TabsRoot = ({
	name,
	value,
	defaultValue,
	onValueChange,
	debug,
	className,
	children,
}: TabsRootProps) => {
	const reactId = useId();
	const groupName = name ?? `tabs${reactId}`;
	const segments = parseSegments(children);
	const values = collectValues(segments);
	const fallbackValue = defaultValue ?? values[0];
	const isControlled = value !== undefined;

	const [internalValue, setInternalValue] = useState(fallbackValue);
	const activeValue = isControlled ? value : internalValue;
	const segmentCount = segments.length;
	const activeSegmentIndex = findActiveSegmentIndex(segments, activeValue);
	const topLeafActive = segments.some(
		(segment) => segment.kind === "tab" && segment.value === activeValue,
	);

	const setActiveValue = (nextValue: string) => {
		if (!isControlled) {
			setInternalValue(nextValue);
		}

		onValueChange?.(nextValue);
	};

	const radioChecked = (tabValue: string) =>
		isControlled ? activeValue === tabValue : undefined;

	const radioDefaultChecked = (tabValue: string) =>
		isControlled ? undefined : fallbackValue === tabValue;

	const trackStyle = {
		"--segments": segmentCount,
		"--active-segment": activeSegmentIndex,
		gridTemplateColumns: `repeat(${segmentCount * 2}, 1fr)`,
	} as CSSProperties;

	if (segmentCount === 0) {
		return null;
	}

	return (
		<div
			className={cn("tabs", className)}
			data-debug={debug ? "true" : undefined}
		>
			<div
				className="track"
				style={trackStyle}
				data-leaf-checked={topLeafActive ? "true" : undefined}
			>
				<div className="indicator" />
				{segments.map((segment) => {
					if (segment.kind === "tab") {
						const inputId = `${reactId}-${segment.value}`;

						return (
							<div
								className="segment"
								key={segment.value}
								style={{ gridColumn: "span 2" }}
							>
								<label htmlFor={inputId}>{segment.label}</label>
								<input
									className="sr-only"
									type="radio"
									name={groupName}
									id={inputId}
									value={segment.value}
									checked={radioChecked(segment.value)}
									defaultChecked={radioDefaultChecked(segment.value)}
									onChange={() => setActiveValue(segment.value)}
								/>
							</div>
						);
					}

					const groupActiveIndex =
						segment.kind === "group"
							? findActiveGroupTabIndex(segment, activeValue)
							: -1;
					const groupExpanded = groupActiveIndex >= 0;

					const groupStyle = {
						"--tabs": segment.tabs.length,
						"--active-tab": groupActiveIndex,
					} as CSSProperties;

					return (
						<div
							className="group"
							key={segment.title}
							style={{ ...groupStyle, gridColumn: "span 2" }}
							data-title={segment.title}
							data-expanded={groupExpanded ? "true" : undefined}
						>
							<div className="indicator" />
							{segment.tabs.map((tab, tabIndex) => {
								const inputId = `${reactId}-${tab.value}`;

								return (
									<div className="tab" key={tab.value}>
										<label htmlFor={inputId}>
											<span>{tab.label}</span>
											{tab.srLabel ? (
												<span className="sr-only">{tab.srLabel}</span>
											) : null}
										</label>
										<input
											className="sr-only"
											type="radio"
											name={groupName}
											id={inputId}
											value={tab.value}
											checked={radioChecked(tab.value)}
											defaultChecked={radioDefaultChecked(tab.value)}
											onChange={() => setActiveValue(tab.value)}
											data-tab-index={tabIndex}
										/>
									</div>
								);
							})}
						</div>
					);
				})}
			</div>
		</div>
	);
};

export const Tabs = Object.assign(TabsRoot, {
	Tab,
	Group,
});
