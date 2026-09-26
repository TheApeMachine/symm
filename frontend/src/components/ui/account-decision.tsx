import { Badge } from "./badge";
import { Flex } from "./flex";
import { Typography } from "./typography";

export interface AccountDecisionProps {
	decision?: string;
	symbol?: string;
	reason?: string;
	phase?: string;
	className?: string;
}

/* AccountDecision displays the account node's latest admission decision and its recorded reason. */
export const AccountDecision = ({
	decision,
	symbol,
	reason,
	phase,
	className,
}: AccountDecisionProps) => (
	<Flex.Column
		className={`gap-3 border-b border-(--line) p-4 ${className ?? ""}`}
	>
		<Typography.Label>Latest execution decision</Typography.Label>
		<Flex.Row className="items-center gap-3">
			<Badge label={decision || "Awaiting a decision"} variant="info" />
			<Typography.Mono>{symbol}</Typography.Mono>
		</Flex.Row>
		<Typography.Mono>{reason}</Typography.Mono>
		<Typography.Mono className="text-(--f4)">{phase}</Typography.Mono>
	</Flex.Column>
);
