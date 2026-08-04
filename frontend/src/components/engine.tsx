import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";

/*
Engine is the run readout.

Each row is painted from the wire key that actually owns it: the sequence
counter from tick, the arbitration state from strategy, and the two volumes from
the batches themselves. The readiness gates are the strategy's own account of
which stages are live, so a run that is still warming up says which stage it is
waiting on rather than reporting a bare "not ready".
*/
const READINESS_GATES = [
	"signals",
	"manifold",
	"resonance",
	"causal",
	"graph",
	"allocation",
	"decisions",
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

export const Engine = () => (
	<Flex.Column className="mx-2 rounded-[3px] border border-(--line) bg-(--sunken) p-2.5 font-mono text-[11px] leading-[1.7]">
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
					<Row label="cand">
						<Flex data-paint="decisions" className="text-(--f1)">
							—
						</Flex>
					</Row>
					<Row label="gates">
						<Flex.Row gap={1} align="center">
							{READINESS_GATES.map((gate) => (
								<span
									key={gate}
									title={gate}
									data-set={`readiness.${gate}`}
									data-set-scale="bool-color"
									data-target="style.background"
									className="size-1.5 rounded-full bg-(--line2)"
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
	</Flex.Column>
);
