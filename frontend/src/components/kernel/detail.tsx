import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import {  focusAtom, kernelDetailAtom, measurementSourcesAtom, symbolsAtom , signals } from "#/collections/app";
import {
	kernelCopy,
	metricLabel,
	sourceMetrics,
} from "#/components/terminal/kernel-meta";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

export const SignalDetail = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);
	const selected = useSelector(kernelDetailAtom, (state) => state);
	const symbols = useSelector(symbolsAtom, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	const source = selected || "cvd";
	const copy =
		source === ""
			? { name: "Signal detail", sub: "", blurb: "" }
			: kernelCopy(source, "");
	const metrics = source === "" ? [] : sourceMetrics(source);

	useEffect(() => {
		if (source === "") return;
		const sourceStore = (signals[source as keyof typeof signals] || signals.cvd);

		const applyState = () => {
			if (!root.current) return;
			const row = sourceStore.state.getLast();

			const set = (q: string, value: string) => {
				const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);
				if (el) el.textContent = value;
			};

			set("symbol", focusSymbol);
			const rowAt = row?.at;
			set(
				"at",
				rowAt === undefined || rowAt === 0n
					? "—"
					: new Date(Number(rowAt / 1000000n)).toISOString().slice(11, 19),
			);
			set(
				"maturity",
				row?.maturity === undefined ? "—" : row.maturity.toFixed(3),
			);
			set("peer", "—");
			set("epoch", String(measurementSourcesAtom.get().length));

			if (row && Array.isArray(row.metrics)) {
				for (const m of row.metrics) {
					if (!m) continue;
					const name = m.name ?? "";
					const raw = m.raw;
					const normalized = m.normalized;

					set(`m:${name}`, raw.toFixed(4));
					const bar = root.current?.querySelector<HTMLElement>(
						`[data-mbar="${name}"]`,
					);
					if (bar instanceof HTMLElement) {
						bar.style.width = `calc(clamp(0, ${Math.min(1, Math.max(0, normalized))}, 1) * 100%)`;
					}
				}
			}
		};

		applyState();
		const subscription = sourceStore.subscribe(applyState);

		return () => {
			subscription.unsubscribe();
		};
	}, [source, focusSymbol]);

	if (source === "") {
		return (
			<Flex.Column className="min-h-0 px-5 py-4.5">
				<Typography.Display size="xl">{copy.name}</Typography.Display>
				<Typography.Paragraph className="mt-3.5 text-[12px] text-(--f3)">
					Select a kernel to inspect its live measurements.
				</Typography.Paragraph>
			</Flex.Column>
		);
	}

	return (
		<Flex.Column
			key={`${source}:${focusSymbol}`}
			ref={root}
			className="min-h-0 overflow-auto px-5 py-4.5"
		>
			<Flex.Row className="items-start justify-between gap-3">
				<Flex.Column className="min-w-0 gap-1">
					<Typography.Display size="xl">{copy.name}</Typography.Display>
					<Typography.Mono size="s" tone="f4">
						{copy.sub}
					</Typography.Mono>
				</Flex.Column>
			</Flex.Row>

			<Typography.Paragraph className="mt-3.5 max-w-prose text-[12px] text-(--f3) leading-relaxed">
				{copy.blurb}
			</Typography.Paragraph>

			<div className="mt-4.5 grid grid-cols-2 gap-x-5.5 gap-y-3">
				{metrics.map((metric: any) => (
					<div key={metric}>
						<Flex.Row justify="between" align="center" className="mb-1.5 gap-2">
							<Typography.Label size="xxs" tone="f3" weight="normal">
								{metricLabel(metric)}
							</Typography.Label>
							<Typography.Mono size="s" tone="f1" data-f={`m:${metric}`}>
								—
							</Typography.Mono>
						</Flex.Row>
						<div className="h-1.5 overflow-hidden rounded-[3px] bg-(--line)">
							<div
								data-mbar={metric}
								className="h-full bg-(--acc) transition-[width] duration-500 ease-out"
								style={{ width: "0%" }}
							/>
						</div>
					</div>
				))}
			</div>

			<div className="mt-5 grid grid-cols-2 gap-x-5.5 gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
				<div className="flex justify-between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Symbol
					</Typography.Label>
					<span data-f="symbol" className="text-(--f1)" />
				</div>
				<div className="flex justify-between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Observed
					</Typography.Label>
					<span data-f="at" className="text-(--f1)" />
				</div>
				<div className="flex justify-between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Maturity
					</Typography.Label>
					<span data-f="maturity" className="text-(--f1)" />
				</div>
				<div className="flex justify-between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Peer
					</Typography.Label>
					<span data-f="peer" className="text-(--f1)" />
				</div>
			</div>

			<div className="mt-3.5 grid grid-cols-2 gap-x-5.5 gap-y-2 font-mono text-xs">
				<div className="flex justify-between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Readings this epoch
					</Typography.Label>
					<span data-f="epoch" className="text-(--f1)" />
				</div>
			</div>

			{symbols.length === 0 ? null : (
				<div className="mt-5">
					<Typography.Label
						size="xxs"
						tone="f3"
						className="mb-2 block tracking-[0.13em]"
					>
						Cross-section · {source} headline
					</Typography.Label>
					<div className="grid grid-cols-12 gap-0.75">
						{symbols.slice(0, 24).map((symbol) => (
							<div
								key={symbol}
								title={`${symbol} · ${source}`}
								className="flex aspect-square items-center justify-center overflow-hidden rounded-xs border border-(--line) font-mono text-[8px] text-(--f3)"
							>
								{symbol.replace(/\/.*$/, "")}
							</div>
						))}
					</div>
				</div>
			)}
		</Flex.Column>
	);
};
