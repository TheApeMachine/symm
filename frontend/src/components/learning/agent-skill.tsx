import { useShellValue } from "#/components/shell-value";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { basis, percent } from "#/components/ui/learning-format";
import { Typography } from "#/components/ui/typography";

/*
AgentSkill states how the learner's completed decisions have gone: the share of
round trips that made money and their mean edge, as the running graph reports
them.
*/
export const AgentSkill = () => {
	const positives = useShellValue("positives");
	const outcomes = useShellValue("outcomes");
	const edgeDefined = useShellValue("edgeDefined");
	const winRate =
		positives !== undefined && outcomes !== undefined && Number(outcomes) > 0
			? Number(positives) / Number(outcomes)
			: undefined;
	const edge = useShellValue("edge");

	return (
		<Flex.Row align="center" gap={6}>
			<Badge label="Model" variant="info" dot />
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Win Rate
				</Typography.Label>
				<Typography.Mono size="lg" tone="f1" data-a="winrate">
					{winRate === undefined ? "—" : percent(winRate)}
				</Typography.Mono>
			</Flex.Column>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Edge
				</Typography.Label>
				<Typography.Mono size="lg" tone="accent" data-a="edge">
					{edgeDefined === true && typeof edge === "number" ? basis(edge) : "—"}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};
