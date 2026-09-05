import { useSelector } from "@tanstack/react-store";
import {
	focusStore,
	onlineStore,
	resonanceTransportDetailStore,
	resonanceTransportStore,
	tickCountStore,
} from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { Balance } from "#/components/balance";
import { Count } from "#/components/count";
import { AgentSkill } from "#/components/learning/agent-skill";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Divider } from "#/components/ui/divider";
import { Flex } from "#/components/ui/flex";
import { Icon } from "#/components/ui/icon";
import { Key } from "#/components/ui/key";
import { Toolbar } from "#/components/ui/toolbar";
import { Typography } from "#/components/ui/typography";

const SymmLogo = () => (
	<svg
		width="22"
		height="22"
		viewBox="0 0 22 22"
		fill="none"
		className="block"
		aria-hidden="true"
	>
		<title>SYMM</title>
		<circle cx="11" cy="11" r="8.5" stroke="var(--acc)" strokeWidth="1.3" />
		<circle cx="11" cy="11" r="3.4" stroke="var(--acc)" strokeWidth="1.3" />
		<path
			d="M11 0.5V5M11 17V21.5M0.5 11H5M17 11H21.5"
			stroke="var(--acc)"
			strokeWidth="1.3"
		/>
	</svg>
);

const TickCounter = () => {
	const tick = useSelector(tickCountStore, (state) => state);

	return (
		<Flex.Row align="center" gap={6}>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Tick
				</Typography.Label>
				<Typography.Mono size="lg" tone="f1" data-tick="true">
					{tick}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};

/*
Rule is the vertical separator between the bar's groups. It is shorter than the
bar so it reads as a division between neighbours rather than as a second edge of
the bar itself, which is what a full-height rule looks like against the toolbar
border.

The height is fixed rather than stretched to fit. Stretching made the rule as
tall as whatever it happened to sit beside, so the separators around the
two-line readouts grew taller than the ones between the badges — the same
component drawing two different lines depending on its neighbours.
*/
const Rule = () => (
	<Divider orientation="vertical" className="h-[18px] self-center" />
);

/*
ResonanceTransportBadge surfaces WebRTC data-channel health next to the websocket
liveness badge so a dead telemetry transport is never mistaken for a quiet
predictive coder. Offline here means the resonance/diagnostics channels are down
or reconnecting — the model may be fine, but no artifacts are arriving.

The label is the transport's name and nothing else: colour carries the state
(red down, orange connecting, green live), and the detail a reconnect leaves —
the countdown, the failing channel — belongs in the hover, where reading it is a
deliberate act rather than a line of chrome that changes width every second.
*/
const ResonanceTransportBadge = () => {
	const status = useSelector(resonanceTransportStore, (state) => state);
	const detail = useSelector(resonanceTransportDetailStore, (state) => state);

	const live = status === "ONLINE";
	const connecting = status === "CONNECTING";
	const state = live ? "live" : connecting ? "connecting" : "offline";
	const variant = live ? "success" : connecting ? "warning" : "error";

	return (
		<Badge
			label="WebRTC"
			variant={variant}
			dot
			pulse={live}
			title={detail ? `WebRTC ${state} · ${detail}` : `WebRTC ${state}`}
		/>
	);
};

export const TerminalTopBar = () => {
	const online = useSelector(onlineStore, (state) => state === "ONLINE");
	const focusSymbol = useSelector(focusStore, (state) => state);
	const { openPalette, openSymbolPalette } = terminalStore.actions;

	return (
		<Toolbar size="lg" className="relative z-5">
			<Toolbar.Group>
				<SymmLogo />
				<span className="font-semibold text-[14px] text-(--f1) tracking-[0.22em]">
					SYMM
				</span>
			</Toolbar.Group>

			<Rule />

			<Toolbar.Group>
				<Badge
					label="WebSocket"
					variant={online ? "success" : "error"}
					dot
					pulse={online}
					title={online ? "WebSocket live" : "WebSocket offline"}
				/>
				<ResonanceTransportBadge />
			</Toolbar.Group>

			<Rule />

			<AgentSkill />

			<Toolbar.Spacer />

			<Toolbar.Group className="gap-3.5">
				<Button
					variant="bare"
					onClick={openSymbolPalette}
					data-symbol={focusSymbol}
					title="Search focused symbol"
				>
					<Badge
						label={focusSymbol}
						variant="warning"
						size="m"
						className="font-mono"
					/>
				</Button>

				<Button variant="outline" size="m" onClick={openPalette}>
					<Icon name="search" size="s" />
					<span className="text-[11px]">Jump to</span>
					<Key>k</Key>
				</Button>

				<Rule />

				<Count />

				<Rule />

				<Balance />
			</Toolbar.Group>

			<Rule />

			<Toolbar.Group>
				<TickCounter />
			</Toolbar.Group>
		</Toolbar>
	);
};
