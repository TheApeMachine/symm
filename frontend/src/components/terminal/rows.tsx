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

export const KernelList = ({
  kernels,
  selectedSource,
  onSelect,
}: {
  kernels: TerminalKernel[];
  selectedSource?: string;
  onSelect?: (source: string) => void;
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
            "block w-full border-stone-800 border-b border-l-2 px-3 py-2.5 text-left transition-colors hover:bg-stone-900/70",
            tone === "good" && "border-l-emerald-300",
            tone === "warn" && "border-l-amber-300",
            tone === "bad" && "border-l-rose-300",
            tone === "muted" && "border-l-stone-600",
            selectedSource === kernel.source && "bg-stone-900",
          )}
        >
          <div className="flex items-center justify-between gap-2">
            <span className="truncate font-semibold text-stone-100 text-xs">
              {kernel.name}
            </span>
            <span
              className={cn(
                "rounded-sm border px-1.5 py-0.5 font-semibold text-[9px] uppercase",
                toneClasses(tone),
              )}
            >
              {kernel.statusLabel}
            </span>
          </div>
          <div className="mt-1 truncate font-mono text-[10px] text-stone-600">
            {kernel.category}
          </div>
          <KernelMeters kernel={kernel} tone={tone} />
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
    <div className="h-1.5 overflow-hidden rounded-sm bg-stone-800">
      <div
        className={cn(
          "h-full",
          tone === "bad"
            ? "bg-rose-300"
            : tone === "warn"
              ? "bg-amber-300"
              : "bg-cyan-300",
        )}
        style={{ width: `${kernel.confidencePercent}%` }}
      />
    </div>
    <span className="w-9 text-right font-mono text-[10px] text-stone-300">
      {kernel.confidenceText}
    </span>
    <span className="w-12 text-right font-mono text-[10px] text-amber-200">
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
