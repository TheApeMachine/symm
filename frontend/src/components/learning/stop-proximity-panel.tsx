import { Meter } from "#/components/ui";
import { Flex } from "#/components/ui/flex";
import {
	buildStopChecks,
	heldVerdict,
	meterTone,
	type Tone,
} from "#/lib/stop-telemetry";
import { cn } from "#/lib/utils";

/*
StopProximityPanel renders the four stop-loss checks the desk refreshes on every
mark — invalidation, liquidity evaporation, trailing, and move expiry — as
meters plus one plain-language verdict. This is the "why is this lot still held,
or about to exit, right now" readout: the operator sees each check's proximity
to firing and the reason behind it, so a held lot is never a black box.

Data is read directly off the raw desk row (stop_state, peak_pnl_pct,
max_lifetime_bars, bars_held). Everything is optional: a lot that has no stop
telemetry yet renders in its waiting state rather than a fabricated zero.
*/
export type StopRow = Record<string, unknown>;

const num = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const readStop = (row: StopRow) => {
	const state = row.stop_state as Record<string, unknown> | null | undefined;
	const stop =
		state && typeof state === "object"
			? (state as Record<string, unknown>)
			: null;

	const current = num(stop?.current_pct) ?? num(row.pnl_pct) ?? null;
	const peak = num(stop?.peak_pct) ?? num(row.peak_pnl_pct) ?? null;

	return {
		invalidation: num(stop?.invalidation) ?? undefined,
		evaporation: num(stop?.evaporation) ?? undefined,
		trailing: num(stop?.trailing) ?? undefined,
		expiry: num(stop?.expiry) ?? undefined,
		current,
		peak,
		barsHeld: num(stop?.bars_held) ?? num(row.bars_held) ?? undefined,
		maxBars: num(stop?.max_bars) ?? num(row.max_lifetime_bars) ?? undefined,
	};
};

export const StopProximityPanel = ({
	row,
	className,
}: {
	row: StopRow;
	className?: string;
}) => {
	const stop = readStop(row);
	const checks = buildStopChecks(stop);
	const verdict = heldVerdict(stop);
	return (
		<Flex.Column
			gap={1}
			className={cn(
				"rounded-xs border border-(--line) bg-(--surface) p-2 font-mono text-[10.5px]",
				className,
			)}
		>
			<Flex.Row align="center" justify="between" className="text-[10px]">
				<span className="font-bold uppercase tracking-wider text-(--f3)">
					Stop proximity
				</span>
				<span
					className="font-bold"
					style={{ color: `var(--${verdictToneColor(verdict.tone)})` }}
					title={verdict.text}
				>
					{verdict.text}
				</span>
			</Flex.Row>

			<Flex.Column gap={1}>
				{checks.map((check) => (
					<Meter
						key={check.id}
						size="s"
						layout="inline"
						variant={meterTone(check.tone)}
						label={check.name}
						value={check.word.short}
						percent={check.percent ?? 0}
						animated
						title={check.word.title}
						labelClassName="text-(--f3)"
						valueClassName="text-(--f1)"
					/>
				))}
			</Flex.Column>
		</Flex.Column>
	);
};

/* The verdict tone is a stop-telemetry Tone; map it to a CSS custom property. */
const verdictToneColor = (tone: Tone): string =>
	tone === "good"
		? "up"
		: tone === "warn"
			? "warn"
			: tone === "bad"
				? "down"
				: tone === "info"
					? "info"
					: "f3";
