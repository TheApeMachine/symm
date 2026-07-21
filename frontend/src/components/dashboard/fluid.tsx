import { Flex } from "@/components/ui/flex";
import type { FluidFieldLayer } from "#/collections/terminal";

export const FluidLegend = ({ layer }: { layer: FluidFieldLayer }) => (
	<Flex.Row className="pointer-events-none absolute bottom-2.5 left-3 gap-3.5 font-mono text-[9px] text-(--f3)">
		<Flex.Row align="center" className="gap-1.5">
			<span className="size-2 rounded-full bg-(--acc) shadow-[0_0_7px_var(--acc)]" />
			focused X–Z occupancy
		</Flex.Row>
		<Flex.Row align="center" className="gap-1.5">
			<span className="size-2 rounded-full bg-info" />
			guidance current
		</Flex.Row>
		{layer !== "Gas" && (
			<Flex.Row align="center" className="gap-1.5">
				<span className="size-2 bg-[linear-gradient(135deg,var(--info),var(--acc))]" />
				|ψ|² coherence
			</Flex.Row>
		)}
		{layer !== "Coherence" && (
			<Flex.Row align="center" className="gap-1.5">
				<span className="size-2 bg-(--down)" />
				gas ρ{layer === "Composite" ? " overlay" : ""}
			</Flex.Row>
		)}
	</Flex.Row>
);
