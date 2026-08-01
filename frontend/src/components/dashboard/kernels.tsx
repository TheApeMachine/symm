import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import type { Measurement } from "#/collections/types";
import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";

type KernelRow = {
	source: string;
	status: string;
	readout: string;
	age: string;
	bar_width: string;
};

type KernelModel = {
	rows: KernelRow[];
};

const headlineMetric = (row: Measurement) => {
	const metric =
		{
			hawkes: "conditional_intensity",
			liquidity: "scarcity_score",
			toxicity: "touch_quantity",
		}[(row.source ?? "").toLowerCase()] ?? "strength";
	const reading =
		Object.entries(row.metrics ?? {})
			.filter(([key]) => key === metric || key.startsWith(`${metric}:`))
			.map(([, value]) => value)
			.sort(
				(left, right) =>
					Number((right as { normalized?: number; raw?: number }).normalized ?? (right as { raw?: number }).raw ?? 0) -
					Number((left as { normalized?: number; raw?: number }).normalized ?? (left as { raw?: number }).raw ?? 0),
			)
			.at(0) ?? row.metrics?.strength;
	const raw = Number(reading?.raw ?? 0);
	const normalized = Number(reading?.normalized ?? raw);

	return {
		metric,
		raw,
		sample: Math.max(0, Math.min(1, Number.isFinite(normalized) ? normalized : 0)),
	};
};

const ageLabel = (at?: string) => {
	if (!at) {
		return "waiting";
	}

	const elapsed = Date.now() - Date.parse(at);

	if (!Number.isFinite(elapsed)) {
		return "waiting";
	}

	if (elapsed < 60_000) {
		return `${Math.max(0, Math.floor(elapsed / 1000))}s`;
	}

	return `${Math.floor(elapsed / 60_000)}m`;
};

const normalizeMeasurements = (value: unknown) =>
	(Array.isArray(value) ? value : value != null ? [value] : []) as Measurement[];

const buildModel = (value: unknown): KernelModel => {
	const rows = normalizeMeasurements(value);
	const focus = appStore.state.focusSymbol;
	const observed = new Set<string>();
	const latest = new Map<string, Measurement>();

	for (const row of rows) {
		if (!row || typeof row.source !== "string") {
			continue;
		}

		observed.add(row.source);

		if (focus !== "" && row.symbol !== focus) {
			continue;
		}

		const current = latest.get(row.source);

		if (!current || Date.parse(current.at) <= Date.parse(row.at)) {
			latest.set(row.source, row);
		}
	}

	appStore.actions.observeSources(observed);

	return {
		rows: appStore.state.kernels.map((source) => {
			const row = latest.get(source);

			if (!row) {
				return {
					source,
					status: "STANDBY",
					readout: "waiting",
					age: "",
					bar_width: "0%",
				};
			}

			const valid = row.validity?.state !== "invalid";
			const { metric, raw, sample } = headlineMetric(row);

			return {
				source,
				status: valid ? "HEALTHY" : "INVALID",
				readout: `${metric} ${Number.isFinite(raw) ? raw.toPrecision(4) : "0"}`,
				age: ageLabel(row.at),
				bar_width: `${Math.round(sample * 100)}%`,
			};
		}),
	};
};

export const KernelList = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);

	return (
		<Component
			registerKey="measurements"
			select="rows"
		>
			{({ ref, className }) => (
				<List
					ref={ref}
					className={cn("min-h-0 flex-1 border-(--line) border-b", className)}
				>
					{kernels.map((kernel, index) => (
						<List.Item
							key={kernel}
							data-index={index}
							className="block border-(--line) border-b px-3 py-2.5"
						>
							<Flex.Row className="items-center justify-between gap-2">
								<Typography.Span
									data-paint="source"
									className="truncate font-semibold text-[12.5px] text-(--f1)"
								>
									{kernel}
								</Typography.Span>
								<Typography.Span
									data-paint="status"
									data-paint-class="HEALTHY:text-(--up) INVALID:text-(--down) STANDBY:text-(--f4)"
									className="shrink-0 rounded-xs border border-(--line2) bg-(--line) px-1.25 py-0.5 font-mono text-[9px] uppercase tracking-[0.07em]"
								>
									STANDBY
								</Typography.Span>
							</Flex.Row>
							<div className="mt-2 h-1 overflow-hidden rounded-xs bg-(--line)">
								<div
									data-set="bar_width"
									data-target="style.width"
									className="h-full w-0 bg-[color-mix(in_srgb,var(--warning)_82%,transparent)]"
								/>
							</div>
							<Flex.Row className="mt-1.5 items-center gap-2">
								<Typography.Span
									data-paint="readout"
									className="min-w-0 flex-1 truncate font-mono text-[10px] text-(--f2)"
								>
									waiting
								</Typography.Span>
								<Typography.Span
									data-paint="age"
									className="w-11.5 shrink-0 text-right font-mono text-[9.5px] text-(--f4)"
								/>
							</Flex.Row>
						</List.Item>
					))}
				</List>
			)}
		</Component>
	);
};
