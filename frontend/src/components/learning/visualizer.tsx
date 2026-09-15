import { useState } from "react";
import { Canvas } from "#/components/ui/canvas";
import { Flex } from "#/components/ui/flex";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";

type VisualInsightMode =
	| "learning"
	| "edge"
	| "actions"
	| "trajectory"
	| "pipeline"
	| "rhythm";

const MODES: Array<{ key: VisualInsightMode; label: string }> = [
	{ key: "learning", label: "Learning" },
	{ key: "edge", label: "Edge" },
	{ key: "actions", label: "Actions" },
	{ key: "trajectory", label: "Trajectory" },
	{ key: "pipeline", label: "Where it stops" },
	{ key: "rhythm", label: "Rhythm" },
];

export const LearningVisualizer = ({
	className,
}: {
	className?: string;
}) => {
	const [mode, setMode] = useState<VisualInsightMode>("learning");

	return (
		<div className="h-full w-full">
			<Canvas
				title="Learning"
				className={`h-full w-full min-h-80 ${className ?? ""}`}
				topRight={
					<Tabs size="xs" className="pointer-events-auto relative z-10">
						{MODES.map((entry) => (
							<Tabs.Tab
								key={entry.key}
								size="xs"
								active={mode === entry.key}
								onClick={() => setMode(entry.key)}
							>
								{entry.label}
							</Tabs.Tab>
						))}
					</Tabs>
				}
				footer={
					<Flex.Row gap={4} align="center">
						<span>
							Edge:{" "}
							<strong className="text-(--acc)" data-metric="edge" data-format="basis">
								0.0 bp
							</strong>
						</span>
						<span>
							Policy choice:{" "}
							<strong className="text-(--acc)" data-metric="action" data-format="action">
								WAIT
							</strong>
						</span>
						<span>
							Learned from:{" "}
							<strong className="text-(--acc)">
								<span data-metric="decisions" data-format="integer">0</span> moves
							</strong>
						</span>
					</Flex.Row>
				}
			>
				<div className="h-full w-full pt-9 pb-7 flex items-center justify-center">
					<Typography.Mono tone="f3">
						Continuous precursor cognition stream active
					</Typography.Mono>
				</div>
			</Canvas>
		</div>
	);
};
