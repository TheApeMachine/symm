import { Flex } from "#/components/ui/flex";
import { strategyStore, useSubscribe } from "#/providers/ws-stores";

const fmtTime = (instant: Date): string => instant.toISOString().slice(11, 19);
const fmtDate = (instant: Date): string => instant.toISOString().slice(0, 10);

export const Clock = () => {
	const root = useSubscribe(strategyStore, (state) => {
		const time = root.current?.querySelector<HTMLElement>("[data-time]");
		const date = root.current?.querySelector<HTMLElement>("[data-date]");

		const envelope =
			state !== null && typeof state === "object" && !Array.isArray(state)
				? (state as { decisions?: unknown })
				: null;
		const first = (Array.isArray(envelope?.decisions) ? envelope.decisions[0] : undefined) as
			| { at?: string }
			| undefined;
		const instant = first?.at === undefined ? null : new Date(first.at);
		const valid = instant !== null && Number.isFinite(instant.getTime());

		if (time instanceof HTMLElement) {
			time.textContent = valid ? `${fmtTime(instant)} UTC` : "—";
		}

		if (date instanceof HTMLElement) {
			date.textContent = valid ? `${fmtDate(instant)} engine clock` : "—";
		}
	});

	return (
		<Flex.Column ref={root}>
			<Flex>
				<span data-time>—</span>
			</Flex>
			<Flex className="text-(--f4)">
				<span data-date>engine clock</span>
			</Flex>
		</Flex.Column>
	);
};
