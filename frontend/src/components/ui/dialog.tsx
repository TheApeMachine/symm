import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { errorAtom } from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

const formatValue = (value: unknown): string => {
	if (value === null) return "null";
	if (value === undefined) return "undefined";
	if (value instanceof Error) return value.message;
	if (typeof value === "object") {
		if (
			"type" in value &&
			typeof (value as { type: unknown }).type === "string"
		) {
			return `${(value as { type: string }).type} event`;
		}
		try {
			return JSON.stringify(value);
		} catch {
			return String(value);
		}
	}
	return String(value);
};

export const Dialog = () => {
	const error = useSelector(errorAtom);
	const [expanded, setExpanded] = useState(false);

	if (!error) return null;

	const source = error.source ? String(error.source) : null;
	const url = error.url ? String(error.url) : null;
	const summary =
		[source, url].filter(Boolean).join(" · ") ||
		formatValue(error.error ?? error);

	return (
		<Flex.Column className="z-40 w-full shrink-0 border-[#5f2d2d] border-b bg-[#1a0f0e] font-mono text-[11px] text-[#f1d7cf]">
			<Flex.Row
				align="center"
				justify="between"
				gap={3}
				className="px-4 py-1.5"
				fullWidth
			>
				<Flex.Row align="center" gap={2} className="min-w-0 flex-1">
					<Badge variant="error" label="BACKEND ERROR" size="xxs" />
					<Typography.Span className="truncate text-[#f1d7cf]">
						{summary}
					</Typography.Span>
				</Flex.Row>

				<Flex.Row align="center" gap={2} className="shrink-0">
					<Button
						variant="quiet"
						size="xs"
						onClick={() => setExpanded((prev) => !prev)}
						className="text-[#d56b61] hover:text-[#f1d7cf]"
					>
						{expanded ? "collapse ▲" : "details ▼"}
					</Button>
					<Button
						variant="quiet"
						size="xs"
						onClick={() => errorAtom.set(null)}
						className="text-(--f3) hover:text-(--f1)"
					>
						dismiss
					</Button>
				</Flex.Row>
			</Flex.Row>

			{expanded ? (
				<Flex.Column
					padding={3}
					gap={1}
					className="max-h-56 overflow-auto border-[#3d1d1d] border-t bg-[#120a09]"
				>
					{Object.entries(error).map(([key, value]) => (
						<Flex.Row key={key} gap={3} align="start">
							<Typography.Span className="w-24 shrink-0 font-semibold text-[#d56b61]">
								{key}:
							</Typography.Span>
							<Typography.Span className="min-w-0 break-all text-[#f1d7cf]">
								{formatValue(value)}
							</Typography.Span>
						</Flex.Row>
					))}
				</Flex.Column>
			) : null}
		</Flex.Column>
	);
};
