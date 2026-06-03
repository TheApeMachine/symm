"use client";

import { Flex } from "#/components/ui/flex";
import { Spinner } from "#/components/ui/spinner";
import { Typography } from "#/components/ui/typography";

/*
LoadablePending is the default pending state. A muted spinner over a
soft "Loading {name}…" caption, sized to fill its parent so it works
as a panel-level placeholder.
*/
export const LoadablePending = ({ name }: { name: string }) => (
	<Flex.Center fullHeight padding={4}>
		<Flex.Column align="center" gap={2}>
			<Spinner className="text-muted-foreground" />
			<Typography.Paragraph variant="muted">
				Loading {name}…
			</Typography.Paragraph>
		</Flex.Column>
	</Flex.Center>
);
