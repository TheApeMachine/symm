import { useSelector } from "@tanstack/react-store";
import { createAtom } from "@tanstack/store";
import { focusAtom, signals } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Tabs } from "#/components/ui/tabs";
import { Component } from "../ui";
import { CognitiveTree } from "./cognitive-tree";
import { ForwardLearningViz } from "./forward-learning-viz";
import { ImpulseMapViz } from "./impulse-map-viz";
import { LifecycleView } from "./lifecycle-view";
import { OpenPositions } from "./open-positions";
import { SequiturGrammarViz } from "./sequitur-grammar-viz";

export type Tab =
	| "positions"
	| "forward"
	| "historical"
	| "cognitive"
	| "sequitur"
	| "impulse";

const TABS: Array<{ key: Tab; label: string }> = [
	{ key: "positions", label: "Open positions" },
	{ key: "forward", label: "Position life-cycle" },
	{ key: "historical", label: "Historical runs" },
	{ key: "cognitive", label: "Cognitive tree" },
	{ key: "sequitur", label: "Sequitur grammar" },
	{ key: "impulse", label: "Impulse map" },
];

export const tabAtom = createAtom<Tab>("positions");

export const LearningDashboard = () => {
	const focus = useSelector(focusAtom, (s) => s);
	const tab = useSelector(tabAtom, (s) => s);

	return (
		<Component
			store={signals.training}
			symbol={focus}
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
					<Flex.Column className="flex-1 min-h-0">
						<OpenPositions />
					</Flex.Column>
				)}

				{tab === "forward" && (
					<Flex.Column className="flex-1 min-h-0">
						<LifecycleView />
					</Flex.Column>
				)}

				{tab === "historical" && (
					<Flex.Column className="flex-1 min-h-0">
						<ForwardLearningViz tapeSource="historical" />
					</Flex.Column>
				)}

				{tab === "cognitive" && (
					<Flex.Column className="flex-1 min-h-0">
						<CognitiveTree />
					</Flex.Column>
				)}

				{tab === "sequitur" && (
					<Flex.Column className="flex-1 min-h-0">
						<SequiturGrammarViz />
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
