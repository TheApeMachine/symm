"use client";

import { CircleAlertIcon } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "#/components/ui/alert";
import { Flex } from "#/components/ui/flex";

/*
LoadableError is the default error state. A muted alert sized to the
panel; consumers override via the error slot when they want a more
specific diagnosis (e.g. "Confirm VITE_ELECTRIC_SHAPE_URL exposes …").
*/
export const LoadableError = ({
	name,
	message,
}: {
	name: string;
	message?: string;
}) => (
	<Flex.Center fullHeight padding={4}>
		<Alert variant="error" className="max-w-lg">
			<CircleAlertIcon />
			<AlertTitle>Error loading {name}</AlertTitle>
			<AlertDescription>
				{message ?? `Could not load ${name}. Check your connection and try again.`}
			</AlertDescription>
		</Alert>
	</Flex.Center>
);
