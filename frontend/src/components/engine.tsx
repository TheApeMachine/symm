import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { Dot } from "#/components/ui/dot";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";

/*
Engine is the run readout.

Each row is painted from the wire key that actually owns it: the sequence
counter from tick, the arbitration state from strategy, and the two volumes from
the batches themselves. The readiness gates are the strategy's own account of
which stages are live, so a run that is still warming up says which stage it is
waiting on rather than reporting a bare "not ready".
*/
const READINESS_GATES = [
	"correlation",
	"cvd",
	"depth_flow",
	"exhaustion",
	"hawkes",
	"lead_lag",
	"liquidity",
	"pump_dump",
	"sentiment",
	"toxicity",
	"categories",
	"manifold",
	"cognition",
	"resonance",
	"causal",
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
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

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

			<Component registerKey="readiness">
				{({ ref }) => (
					<Flex.Column ref={ref} data-scope="symbol" data-filter={focusSymbol}>
						<Row label="gates">
							<Flex.Row gap={1} align="center">
								{READINESS_GATES.map((gate) => (
									<Dot
										key={gate}
										variant="disabled"
										title={gate}
										data-set={gate}
										data-set-scale="bool-color"
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
