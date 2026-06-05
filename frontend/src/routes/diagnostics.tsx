import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useEffect, useState } from "react";

/*
Signal Insight — reads the raw diagnostic dumps the signals write (runs/<name>_raw.jsonl)
through the backend's /api/dumps and /api/analyze endpoints, and renders the
analyzer's plain-language verdict on whether each signal behaves sensibly: a steady
baseline with real excursions (HEALTHY), smooth but inert (FLAT), flipping around
its mean every tick (FLICKERING), or carrying no information (DEAD).

It deliberately uses plain SVG/HTML rather than the SciChart pipeline so it stays
dependency-light and cannot break the rest of the dashboard's build.
*/

type Bin = { lo: number; hi: number; count: number };
type CategoryCount = { value: string; count: number };

type FieldReport = {
	name: string;
	kind: string;
	count: number;
	min: number;
	p1: number;
	median: number;
	mean: number;
	p90: number;
	p99: number;
	max: number;
	std: number;
	lag1_autocorr: number;
	mean_crossing_rate: number;
	jump_rate: number;
	baseline_occupancy: number;
	zero_fraction: number;
	histogram?: Bin[];
	distinct: number;
	switch_rate: number;
	top?: CategoryCount[];
	verdict: string;
	notes?: string[];
};

type Report = {
	signal: string;
	file: string;
	rows: number;
	skipped: number;
	truncated: boolean;
	fields: FieldReport[];
	headline: string;
	generated_at: string;
};

type DumpInfo = {
	signal: string;
	file: string;
	bytes: number;
	modified: string;
};

// The backend serves the analyzer API on the same host/port as the websocket.
const apiBase = (() => {
	const ws =
		(import.meta.env.VITE_SYMM_WS_URL as string | undefined)?.trim() ||
		"ws://127.0.0.1:8765/ws";

	try {
		const url = new URL(ws);
		url.protocol = url.protocol === "wss:" ? "https:" : "http:";
		url.pathname = "";
		url.search = "";
		return url.toString().replace(/\/$/, "");
	} catch {
		return "http://127.0.0.1:8765";
	}
})();

const VERDICT_TONE: Record<string, string> = {
	HEALTHY: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
	OK: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
	FLAT: "bg-amber-500/15 text-amber-400 border-amber-500/30",
	NOISY: "bg-amber-500/15 text-amber-400 border-amber-500/30",
	FLICKERING: "bg-rose-500/15 text-rose-400 border-rose-500/30",
	UNSTABLE: "bg-rose-500/15 text-rose-400 border-rose-500/30",
	DEAD: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
	CONSTANT: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
	EMPTY: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
};

const verdictTone = (verdict: string) =>
	VERDICT_TONE[verdict] ?? "bg-sky-500/15 text-sky-400 border-sky-500/30";

const fmt = (value: number): string => {
	if (!Number.isFinite(value)) {
		return "—";
	}

	const abs = Math.abs(value);

	if (abs !== 0 && (abs >= 1e5 || abs < 1e-3)) {
		return value.toExponential(2);
	}

	return value
		.toFixed(4)
		.replace(/\.?0+$/, "")
		.replace(/^-0$/, "0");
};

const fmtBytes = (bytes: number): string => {
	if (bytes < 1024) {
		return `${bytes} B`;
	}

	const units = ["KB", "MB", "GB"];
	let size = bytes / 1024;
	let unit = 0;

	while (size >= 1024 && unit < units.length - 1) {
		size /= 1024;
		unit += 1;
	}

	return `${size.toFixed(1)} ${units[unit]}`;
};

const Histogram = ({ bins }: { bins: Bin[] }) => {
	if (!bins || bins.length === 0) {
		return null;
	}

	const width = 280;
	const height = 60;
	const max = Math.max(1, ...bins.map((bin) => bin.count));
	const barWidth = width / bins.length;

	return (
		<svg
			viewBox={`0 0 ${width} ${height}`}
			className="h-16 w-full text-sky-400"
			preserveAspectRatio="none"
			role="img"
			aria-label="distribution histogram"
		>
			{bins.map((bin, index) => {
				const barHeight = (bin.count / max) * (height - 2);

				return (
					<rect
						key={`${bin.lo}-${index}`}
						x={index * barWidth}
						y={height - barHeight}
						width={Math.max(0.5, barWidth - 0.5)}
						height={barHeight}
						fill="currentColor"
						opacity={0.85}
					/>
				);
			})}
		</svg>
	);
};

const Metric = ({ label, value }: { label: string; value: string }) => (
	<div className="flex flex-col">
		<span className="text-[10px] uppercase tracking-wide text-muted-foreground">
			{label}
		</span>
		<span className="font-mono text-xs">{value}</span>
	</div>
);

const CategoryBars = ({ top }: { top: CategoryCount[] }) => {
	const max = Math.max(1, ...top.map((entry) => entry.count));

	return (
		<div className="flex flex-col gap-1">
			{top.map((entry) => (
				<div key={entry.value} className="flex items-center gap-2">
					<span className="w-28 shrink-0 truncate font-mono text-[11px] text-muted-foreground">
						{entry.value}
					</span>
					<div className="h-2.5 flex-1 overflow-hidden rounded-full bg-muted">
						<div
							className="h-full rounded-full bg-sky-400"
							style={{ width: `${(entry.count / max) * 100}%` }}
						/>
					</div>
					<span className="w-12 shrink-0 text-right font-mono text-[11px] text-muted-foreground">
						{entry.count}
					</span>
				</div>
			))}
		</div>
	);
};

const FieldCard = ({ field }: { field: FieldReport }) => {
	const isNumeric = field.kind === "numeric";

	return (
		<div className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4">
			<div className="flex items-center justify-between gap-2">
				<div className="flex items-center gap-2">
					<span className="font-mono text-sm font-medium">{field.name}</span>
					<span className="text-[10px] uppercase tracking-wide text-muted-foreground">
						{field.kind}
					</span>
				</div>
				<span
					className={`rounded-full border px-2 py-0.5 text-[11px] font-semibold ${verdictTone(
						field.verdict,
					)}`}
				>
					{field.verdict}
				</span>
			</div>

			{field.notes && field.notes.length > 0 ? (
				<p className="text-xs text-muted-foreground">{field.notes.join(" · ")}</p>
			) : null}

			{isNumeric ? (
				<>
					<Histogram bins={field.histogram ?? []} />
					<div className="grid grid-cols-4 gap-2">
						<Metric label="median" value={fmt(field.median)} />
						<Metric label="mean" value={fmt(field.mean)} />
						<Metric label="p99" value={fmt(field.p99)} />
						<Metric label="max" value={fmt(field.max)} />
						<Metric label="std" value={fmt(field.std)} />
						<Metric label="autocorr" value={fmt(field.lag1_autocorr)} />
						<Metric
							label="flips/tick"
							value={fmt(field.mean_crossing_rate)}
						/>
						<Metric
							label="at baseline"
							value={`${Math.round(field.baseline_occupancy * 100)}%`}
						/>
					</div>
				</>
			) : (
				<>
					<CategoryBars top={field.top ?? []} />
					<div className="grid grid-cols-3 gap-2">
						<Metric label="distinct" value={String(field.distinct)} />
						<Metric
							label="switches/tick"
							value={fmt(field.switch_rate)}
						/>
						<Metric label="count" value={String(field.count)} />
					</div>
				</>
			)}
		</div>
	);
};

const DiagnosticsPage = () => {
	const [dumps, setDumps] = useState<DumpInfo[]>([]);
	const [selected, setSelected] = useState<string | null>(null);
	const [report, setReport] = useState<Report | null>(null);
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState<string | null>(null);

	const loadDumps = useCallback(async () => {
		setError(null);

		try {
			const response = await fetch(`${apiBase}/api/dumps`);

			if (!response.ok) {
				throw new Error(`dumps: ${response.status}`);
			}

			const data = (await response.json()) as { dumps: DumpInfo[] };
			setDumps(data.dumps ?? []);
		} catch (cause) {
			setError(
				`Could not reach the analyzer API at ${apiBase}. Is the engine running? (${String(
					cause,
				)})`,
			);
		}
	}, []);

	const analyze = useCallback(async (signal: string) => {
		setSelected(signal);
		setLoading(true);
		setError(null);
		setReport(null);

		try {
			const response = await fetch(
				`${apiBase}/api/analyze?signal=${encodeURIComponent(signal)}`,
			);
			const data = (await response.json()) as Report & { error?: string };

			if (!response.ok || data.error) {
				throw new Error(data.error ?? `analyze: ${response.status}`);
			}

			setReport(data);
		} catch (cause) {
			setError(String(cause));
		} finally {
			setLoading(false);
		}
	}, []);

	useEffect(() => {
		void loadDumps();
	}, [loadDumps]);

	return (
		<div className="flex h-full min-h-0 w-full flex-col gap-4">
			<div className="flex flex-wrap items-center justify-between gap-3">
				<div className="flex flex-col">
					<h1 className="text-lg font-semibold">Signal Insight</h1>
					<p className="text-xs text-muted-foreground">
						Raw signal diagnostics — does each signal actually make sense?
					</p>
				</div>
				<button
					type="button"
					onClick={() => {
						void loadDumps();
						if (selected) {
							void analyze(selected);
						}
					}}
					className="rounded-lg border border-border bg-card px-3 py-1.5 text-xs hover:bg-muted"
				>
					Refresh
				</button>
			</div>

			{dumps.length === 0 ? (
				<div className="rounded-xl border border-dashed border-border p-6 text-sm text-muted-foreground">
					No raw dumps found under <code>runs/</code>. Enable one with{" "}
					<code>signals.&lt;name&gt;.raw_dump: true</code> (or the global{" "}
					<code>signals.raw_dump: true</code>) and run the engine to produce{" "}
					<code>runs/&lt;name&gt;_raw.jsonl</code>.
				</div>
			) : (
				<div className="flex flex-wrap gap-2">
					{dumps.map((dump) => (
						<button
							key={dump.signal}
							type="button"
							onClick={() => void analyze(dump.signal)}
							className={`flex flex-col items-start rounded-lg border px-3 py-2 text-left transition-colors ${
								selected === dump.signal
									? "border-sky-500/50 bg-sky-500/10"
									: "border-border bg-card hover:bg-muted"
							}`}
						>
							<span className="font-mono text-sm">{dump.signal}</span>
							<span className="text-[10px] text-muted-foreground">
								{fmtBytes(dump.bytes)}
							</span>
						</button>
					))}
				</div>
			)}

			{error ? (
				<div className="rounded-xl border border-rose-500/30 bg-rose-500/10 p-4 text-sm text-rose-400">
					{error}
				</div>
			) : null}

			{loading ? (
				<div className="text-sm text-muted-foreground">Analyzing…</div>
			) : null}

			{report ? (
				<div className="flex min-h-0 flex-1 flex-col gap-3 overflow-auto">
					<div className="rounded-xl border border-border bg-card p-4">
						<p className="text-sm font-medium">{report.headline}</p>
						<p className="mt-1 text-xs text-muted-foreground">
							{report.file}
							{report.truncated
								? ` · truncated to first ${report.rows.toLocaleString()} rows`
								: ""}
							{report.skipped > 0 ? ` · ${report.skipped} malformed lines` : ""}
						</p>
					</div>

					<div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
						{report.fields.map((field) => (
							<FieldCard key={field.name} field={field} />
						))}
					</div>
				</div>
			) : null}
		</div>
	);
};

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsPage,
});
