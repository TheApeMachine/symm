import { Component } from "#/components/ui/component";
import { Dot } from "#/components/ui/dot";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";

/*
Engine is the run readout.

Each row is painted from the wire key that actually owns it: the sequence
counter from tick, the arbitration state from strategy, and the two volumes from
the batches themselves. The activity row shows whether each signal, logic module,
and the planner is currently running or finished its last cycle.
*/
const ACTIVITY_MODULES = [
	"correlation",
	"cvd",
	"depthflow",
	"exhaustion",
	"hawkes",
	"leadlag",
	"liquidity",
	"pumpdump",
	"sentiment",
	"toxicity",
	"category",
	"manifold",
	"resonance",
	"causal",
	"cognition",
	"graph",
	"planner",
] as const;

const Row = ({
	label,
	children,
}: {
	label: string;
	children: React.ReactNode;
}) => (
	<Flex.Row justify="between" align="center" className="gap-2">
		<Flex className="shrink-0 text-(--f4)">{label}</Flex>
		{children}
	</Flex.Row>
);

export const Engine = () => {
	return (
		<Panel size="bare" className="p-2.5 font-mono text-[11px] leading-[1.7]">
			<Component registerKey="tick">
				{({ ref }) => (
					<Flex.Column ref={ref}>
						<Row label="seq">
							<Flex data-paint="count" className="text-(--f1)">
								—
							</Flex>
						</Row>
					</Flex.Column>
				)}
			</Component>

			{/*
			The arbitration phase and the candidate count belong to the strategy
			frame, not to readiness — readiness only carries the stage gates. Asking
			readiness for them is why both rows sat on their placeholder.
		*/}
			<Component registerKey="strategy">
				{({ ref }) => (
					<Flex.Column ref={ref}>
						<Row label="phase">
							<Flex
								data-paint="outcome"
								className="min-w-0 truncate text-(--acc)"
							>
								—
							</Flex>
						</Row>
					</Flex.Column>
				)}
			</Component>

			<Component registerKey="strategy" select="decisions">
				{({ ref }) => (
					<Flex.Column ref={ref}>
						<Row label="cand">
							<Flex data-paint="length" className="text-(--f1)">
								—
							</Flex>
						</Row>
					</Flex.Column>
				)}
			</Component>

			<Component registerKey="activity">
				{({ ref }) => (
					<Flex.Column ref={ref}>
						<Row label="gates">
							<Flex.Row gap={1} align="center">
								{ACTIVITY_MODULES.map((gate) => (
									<Dot
										key={gate}
										variant="disabled"
										title={gate}
										data-set={gate}
										data-set-scale="activity-color"
										data-target="style.background"
									/>
								))}
							</Flex.Row>
						</Row>
					</Flex.Column>
				)}
			</Component>

			<Component registerKey="measurements">
				{({ ref }) => (
					<Flex.Column ref={ref}>
						<Row label="meas">
							<Flex data-paint="length" className="text-(--f1)">
								—
							</Flex>
						</Row>
					</Flex.Column>
				)}
			</Component>

			<Component registerKey="positions">
				{({ ref }) => (
					<Flex.Column ref={ref}>
						<Row label="open">
							<Flex data-paint="length" className="text-(--f1)">
								—
							</Flex>
						</Row>
					</Flex.Column>
				)}
			</Component>
		</Panel>
	);
};
