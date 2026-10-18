import { cn } from "#/lib/utils";
import { Badge } from "./badge";
import { Flex } from "./flex";

export interface TokenRibbonProps {
	/* Each token is the list of sectors it is made of, oldest token first. */
	tokens: number[][];
	/* Slots the sequence fills before it can match. */
	capacity?: number;
	label?: (sector: number) => string;
	className?: string;
}

/*
TokenRibbon draws a sequence as extra-small badges. The latest token is
info; earlier tokens and empty slots are disabled.
*/
export const TokenRibbon = ({
	tokens,
	capacity,
	label,
	className,
}: TokenRibbonProps) => {
	const slots = Math.max(capacity ?? 0, tokens.length);

	if (slots === 0) {
		return <Badge size="xs" variant="disabled" label="no tokens yet" />;
	}

	const seen = new Map<string, number>();
	const keyFor = (id: string) => {
		const count = (seen.get(id) ?? 0) + 1;
		seen.set(id, count);
		return `${id}#${count}`;
	};

	return (
		<Flex.Row align="center" wrap="wrap" gap={1} className={cn(className)}>
			{tokens.map((token, index) => {
				const text = token
					.map((sector) => (label ? label(sector) : sector))
					.join(" ");

				return (
					<Badge
						key={keyFor(token.join("."))}
						size="xs"
						variant={index === tokens.length - 1 ? "info" : "disabled"}
						label={text}
					/>
				);
			})}
			{Array.from({ length: slots - tokens.length }, () => (
				<Badge key={keyFor("empty")} size="xs" variant="disabled" label="—" />
			))}
		</Flex.Row>
	);
};
