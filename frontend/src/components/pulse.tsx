import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

/*
Pulse is the run strip above the dashboard.

Every reading on it is owned by a different wire key, so the strip is a row of
small Components rather than one: the sequence counter comes from tick, the
arbitration phase and its candidate count from strategy, the batch sizes from
the batches themselves, and the fluid occupancy from the manifold frame. Binding
them all to one key was why most of the strip never left its placeholder — tick
only ever carries a count.
*/
const Reading = ({
	label,
	registerKey,
	select,
	bind,
	row,
	accent = false,
}: {
	label?: string;
	registerKey: string;
	select?: string;
	bind: string;
	/*
		A batch key answers two different questions. "length" is asked of the batch
		itself, so it must not name a row — pinning data-index would resolve the
		batch to one frame and the size would read as absent.
	*/
	row?: number;
	accent?: boolean;
}) => (
	<Component registerKey={registerKey} select={select}>
		{({ ref }) => (
			<Flex.Row
				ref={ref}
				data-index={row === undefined ? undefined : String(row)}
				align="center"
				gap={1}
			>
				{label ? <span>{label}</span> : null}
				<span
					data-paint={bind}
					className={cn(
						accent ? "text-(--acc)" : "font-semibold text-(--f1)",
						"tabular-nums",
					)}
				>
					—
				</span>
			</Flex.Row>
		)}
	</Component>
);

export const Pulse = () => (
	<Flex.Row
		align="center"
		gap={4}
		className="h-8 shrink-0 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)"
	>
		<Reading registerKey="tick" bind="count" />
		<Reading label="phase" registerKey="strategy" bind="outcome" accent />
		<Reading label="meas" registerKey="measurements" bind="length" />
		<Reading
			label="cand"
			registerKey="strategy"
			select="decisions"
			bind="length"
		/>
		<Reading label="open" registerKey="positions" bind="length" />
		<Reading label="fluid" registerKey="manifold" bind="RhoOccupied" row={0} />
	</Flex.Row>
);
