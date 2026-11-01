import { useSelector } from "@tanstack/react-store";
import { confidenceAtom } from "#/collections/confidence";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";

/*
Compact top-bar strip: which of the three concurrent modes is live, and
whether real is armed or halted on paper edge.
*/
export const ModeStrip = () => {
	const confidence = useSelector(confidenceAtom, (state) => state);
	const real = confidence.real;
	const realLabel = !confidence.real_ceiling
		? "Real off"
		: real.armed
			? "Real armed"
			: "Real halted";
	const realVariant = !confidence.real_ceiling
		? "disabled"
		: real.armed
			? "success"
			: "warning";

	return (
		<Flex.Row align="center" gap={4} data-modes="strip" title="Concurrent trading modes">
			<Badge label="Virtual" variant="info" size="s" />
			<Badge label="Paper" variant="success" size="s" />
			<Badge label={realLabel} variant={realVariant} size="s" />
		</Flex.Row>
	);
};
