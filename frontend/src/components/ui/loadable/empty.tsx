"use client";

import { InboxIcon } from "lucide-react";
import { Empty } from "#/components/ui/empty";
import { Flex } from "#/components/ui/flex";

/*
LoadableEmpty is the default empty state. Generic copy and an inbox
icon — most domains will pass a richer empty slot, but this keeps the
unstyled fallback consistent across the app.
*/
export const LoadableEmpty = ({ name }: { name: string }) => (
	<Flex.Center fullHeight padding={4}>
		<Empty>
			<Empty.Header>
				<Empty.Media variant="icon">
					<InboxIcon className="size-5" />
				</Empty.Media>
				<Empty.Title>No {name} yet</Empty.Title>
				<Empty.Description>
					Once {name} are added, they will appear here.
				</Empty.Description>
			</Empty.Header>
		</Empty>
	</Flex.Center>
);
