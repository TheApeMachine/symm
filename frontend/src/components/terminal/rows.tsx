import type {
  TerminalDecisionRow,
  TerminalKernel,
  TerminalModel,
  TerminalPositionRow,
} from "#/components/terminal/model";
import { toneClasses, type TerminalTone } from "#/components/terminal/tone";
import { cn } from "#/lib/utils";

const kernelTone = (kernel: TerminalKernel): TerminalTone => {
  if (kernel.status === "healthy") {
    return "good";
  }

  if (kernel.status === "calibrating") {
    return "warn";
  }

  if (kernel.status === "fault" || kernel.status === "stale") {
    return "bad";
  }

  return "muted";
};

const verdictTone = (verdict: TerminalDecisionRow["verdict"]): TerminalTone => {
  if (verdict === "allow") {
    return "good";
  }

  if (verdict === "in-play") {
    return "info";
  }

  return "bad";
};

const kernelTrace = (kernel: TerminalKernel): number[] => {
  const confidence = kernel.confidencePercent / 100;
  const surprise = kernel.surprisePercent / 100;
  const health = kernel.healthPercent / 100;

  return Array.from({ length: 30 }, (_, index) => {
    const wave = Math.sin(index / 4 + confidence * Math.PI);
    const drift = index / 36;

    return Math.max(
      0.04,
      Math.min(
        0.96,
        confidence * 0.55 +
          surprise * 0.25 +
          health * 0.2 +
          wave * 0.05 +
          drift * 0.08,
      ),
    );
  });
};

const sparkPath = (values: number[], width: number, height: number): string =>
  values
    .map((value, index) => {
      const x = (index / Math.max(values.length - 1, 1)) * width;
      const y = height - Math.max(0, Math.min(1, value)) * height;

      return `${index === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");

const KernelSparkline = ({ kernel }: { kernel: TerminalKernel }) => {
  const values = kernelTrace(kernel);
  const line = sparkPath(values, 116, 24);
  const area = `${line} L116 24 L0 24 Z`;

  return (
    <svg
      aria-hidden="true"
      className="h-7 w-full overflow-visible"
      viewBox="0 0 116 24"
      preserveAspectRatio="none"
    >
      <path d={area} fill="rgba(232,163,61,0.18)" />
      <path d={line} fill="none" stroke="var(--acc)" strokeWidth="1.35" />
    </svg>
  );
};

export const KernelList = ({
  kernels,
  selectedSource,
  onSelect,
  compact = false,
}: {
  kernels: TerminalKernel[];
  selectedSource?: string;
  onSelect?: (source: string) => void;
  compact?: boolean;
}) => (
  <div className="min-h-0 overflow-auto">
    {kernels.map((kernel) => {
      const tone = kernelTone(kernel);

      return (
        <button
          type="button"
          key={kernel.source}
          onClick={() => onSelect?.(kernel.source)}
          className={cn(
            "block w-full border-[var(--line)] border-b border-l-2 px-3 text-left transition-colors hover:bg-[var(--raised)]",
            compact ? "py-3" : "py-2.5",
            tone === "good" && "border-l-[var(--up)]",
            tone === "warn" && "border-l-[var(--acc)]",
            tone === "bad" && "border-l-[var(--down)]",
            tone === "muted" && "border-l-[var(--line2)]",
            selectedSource === kernel.source && "bg-[var(--raised)]",
          )}
        >
          <div className="flex items-center justify-between gap-2">
            <span className="truncate font-semibold text-[var(--f1)] text-xs">
              {kernel.name}
            </span>
            {compact ? (
              <span
                className={cn(
                  "size-1.5 shrink-0 rounded-full",
                  tone === "good" && "bg-[var(--up)]",
                  tone === "warn" && "bg-[var(--acc)]",
                  tone === "bad" && "bg-[var(--down)]",
                  tone === "muted" && "bg-[var(--f4)]",
                )}
              />
            ) : (
              <span
                className={cn(
                  "rounded-sm border px-1.5 py-0.5 font-semibold text-[9px] uppercase",
                  toneClasses(tone),
                )}
              >
                {kernel.statusLabel}
              </span>
            )}
          </div>
          <div className="mt-1 truncate font-mono text-[10px] text-[var(--f4)]">
            {compact
              ? `${kernel.confidencePercent}% conf · ${kernel.statusLabel}`
              : kernel.category}
          </div>
          {compact ? null : (
            <>
              <div className="mt-2">
                <KernelSparkline kernel={kernel} />
              </div>
              <KernelMeters kernel={kernel} tone={tone} />
            </>
          )}
        </button>
      );
    })}
  </div>
);

const KernelMeters = ({
  kernel,
  tone,
}: {
  kernel: TerminalKernel;
  tone: TerminalTone;
}) => (
  <div className="mt-2 grid grid-cols-[1fr_auto_auto] items-center gap-2">
    <div className="h-1.5 overflow-hidden rounded-sm bg-[var(--line)]">
      <div
        className={cn(
          "h-full",
          tone === "bad"
            ? "bg-[var(--down)]"
            : tone === "warn"
              ? "bg-[var(--acc)]"
              : "bg-[var(--info)]",
        )}
        style={{ width: `${kernel.confidencePercent}%` }}
      />
    </div>
    <span className="w-9 text-right font-mono text-[10px] text-[var(--f2)]">
      {kernel.confidenceText}
    </span>
    <span className="w-12 text-right font-mono text-[10px] text-[var(--acc)]">
      {kernel.surpriseText}
    </span>
  </div>
);

export const DecisionRows = ({
  decisions,
}: {
  decisions: TerminalDecisionRow[];
}) => (
  <div className="min-h-0 overflow-auto">
    <table className="w-full border-collapse text-left text-[11px]">
      <thead className="sticky top-0 bg-stone-950 text-stone-600">
        <tr>
          <th className="px-3 py-1.5 font-semibold uppercase">Symbol</th>
          <th className="px-2 py-1.5 text-right font-semibold uppercase">
            Score
          </th>
          <th className="px-2 py-1.5 font-semibold uppercase">Verdict</th>
        </tr>
      </thead>
      <tbody>
        {decisions.map((decision) => (
          <tr key={decision.key} className="border-stone-800 border-t">
            <td className="px-3 py-1.5 font-mono font-semibold text-stone-100">
              {decision.symbol}
              <div className="font-normal text-[9px] text-stone-600">
                {decision.source}
              </div>
            </td>
            <td className="px-2 py-1.5 text-right font-mono text-stone-300">
              {decision.scoreText}
            </td>
            <td className="px-2 py-1.5">
              <span
                className={cn(
                  "rounded-sm border px-1.5 py-0.5 font-semibold text-[9px] uppercase",
                  toneClasses(verdictTone(decision.verdict)),
                )}
              >
                {decision.verdict}
              </span>
              <div className="mt-1 max-w-32 truncate text-[9px] text-stone-600">
                {decision.why}
              </div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  </div>
);

export const PositionRows = ({
  positions,
}: {
  positions: TerminalPositionRow[];
}) => (
  <div className="min-h-0 overflow-auto p-2">
    {positions.length === 0 ? (
      <div className="px-2 py-8 text-center font-mono text-stone-600 text-xs">
        No open positions
      </div>
    ) : (
      positions.map((position) => (
        <div
          key={position.key}
          className="mb-1.5 rounded-md border border-stone-800 bg-black/25 px-2 py-2"
        >
          <div className="flex items-center justify-between gap-2">
            <span className="font-mono font-semibold text-stone-100 text-xs">
              {position.symbol}
            </span>
            <span
              className={cn(
                "font-mono font-semibold text-xs",
                position.profitable ? "text-emerald-300" : "text-rose-300",
              )}
            >
              {position.pnlText}
            </span>
          </div>
          <div className="mt-1 flex items-center justify-between gap-2 font-mono text-[10px] text-stone-600">
            <span className="truncate">{position.quantityText}</span>
            <span>{position.pnlPercentText}</span>
          </div>
          <div className="mt-1 truncate font-mono text-[10px] text-stone-500">
            {position.priceText}
          </div>
        </div>
      ))
    )}
  </div>
);

export const AuditRows = ({ rows }: { rows: TerminalModel["auditRows"] }) => (
  <div className="min-h-0 overflow-auto">
    {rows.length === 0 ? (
      <div className="px-3 py-8 text-center font-mono text-stone-600 text-xs">
        No audit events
      </div>
    ) : (
      rows.slice(0, 30).map((row) => (
        <div key={row.key} className="border-stone-800 border-b px-3 py-2">
          <div className="flex items-start justify-between gap-3">
            <span className="truncate font-semibold text-stone-200 text-xs">
              {row.reason || row.event}
            </span>
            <span className="shrink-0 font-mono text-[9px] text-stone-600">
              #{row.seq}
            </span>
          </div>
          <div className="mt-1 truncate font-mono text-[9px] text-stone-600">
            {row.symbol || row.source || row.ts}
          </div>
          {row.summary ? (
            <div className="mt-1 truncate text-[10px] text-stone-500">
              {row.summary}
            </div>
          ) : null}
        </div>
      ))
    )}
  </div>
);
