import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";

export const AgentSkill = () => {
	return (
		<Flex.Row align="center" gap={6}>
			<Badge label="Agent" variant="info" dot pulse title="Autonomous trading agent active" />
		</Flex.Row>
	);
};
