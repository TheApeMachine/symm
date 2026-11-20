import { useSelector } from "@tanstack/react-store";
import { confidenceAtom } from "#/collections/confidence";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";

/*
Compact top-bar strip: which books this process actually runs, and real's
gate (armed / halted / off) from the hub confidence frame.
*/
export const ModeStrip = () => {
	const confidence = useSelector(confidenceAtom, (state) => state);
	const virtualOn = confidence.virtual.active === true;
	const paperOn = confidence.paper.active === true;
	const real = confidence.real;
	const realOn = confidence.real_ceiling === true && real.active === true;
	const realVariant = !realOn
		? "disabled"
		: real.armed
			? "success"
			: "warning";

	return (
		<Flex.Row
			align="center"
			gap={4}
			data-modes="strip"
			title="Concurrent trading modes"
		>
			<Badge
				label="Virtual"
				variant={virtualOn ? "info" : "disabled"}
				size="s"
			/>
			<Badge
				label="Paper"
				variant={paperOn ? "success" : "disabled"}
				size="s"
			/>
			<Badge label="Real" variant={realVariant} size="s" />
		</Flex.Row>
	);
};
