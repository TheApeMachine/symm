import { atoms, signals } from "#/collections/app";
import { CognitiveTree } from "#/components/learning/cognitive-tree";
import { ImpulseMapViz } from "#/components/learning/impulse-map-viz";
import { LifecycleView } from "#/components/learning/lifecycle-view";
import { OpenPositions } from "#/components/learning/open-positions";
import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { Tabs } from "#/components/ui/tabs";
import { createFileRoute } from "@tanstack/react-router";
import { createAtom, useSelector } from "@tanstack/react-store";

type TabKey = "positions" | "forward" | "cognitive" | "impulse";

const TABS: { key: TabKey; label: string }[] = [
	{ key: "positions", label: "Positions" },
	{ key: "forward", label: "Forward" },
	{ key: "cognitive", label: "Cognitive" },
	{ key: "impulse", label: "Impulse" },
];

const tabAtom = createAtom<TabKey>("positions");

const RouteComponent = () => {
	const focus = useSelector(atoms.focus, (s) => s);
	const tab = useSelector(tabAtom, (s) => s);

	return (
		<Component
			store={signals.workspace}
			symbol="aggregate"
			className="h-full min-h-0 w-full flex flex-col"
		>
			<Flex.Column className="h-full min-h-0 w-full bg-(--bg) text-(--f2) font-mono">
				<Flex.Row
					align="center"
					justify="between"
					className="border-(--line) border-b bg-(--surface) px-3 py-1 shrink-0"
				>
					<Tabs size="m" className="flex-wrap">
						{TABS.map((entry) => (
							<Tabs.Tab
								key={entry.key}
								size="m"
								active={tab === entry.key}
								onClick={() => tabAtom.set(entry.key)}
							>
								{entry.label}
							</Tabs.Tab>
						))}
					</Tabs>
				</Flex.Row>

				{tab === "positions" && (
					<Flex.Column className="flex-1 min-h-0 w-full min-w-0">
						<OpenPositions />
					</Flex.Column>
				)}

				{tab === "forward" && (
					<Flex.Column className="flex-1 min-h-0">
						<LifecycleView />
					</Flex.Column>
				)}

				{tab === "cognitive" && (
					<Flex.Column className="flex-1 min-h-0">
						<CognitiveTree />
					</Flex.Column>
				)}

				{tab === "impulse" && (
					<Flex.Column className="flex-1 min-h-0">
						<ImpulseMapViz symbol={focus} />
					</Flex.Column>
				)}
			</Flex.Column>
		</Component>
	);
};

export const Route = createFileRoute("/learning")({
	component: RouteComponent,
});
