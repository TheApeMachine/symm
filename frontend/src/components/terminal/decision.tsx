import type {
  TerminalDecisionRow,
  TerminalKernel,
  TerminalModel,
} from "#/components/terminal/model";
import { clamp, fixed } from "#/components/terminal/decision-format";
import { DecisionSideRail } from "#/components/terminal/decision-side";
import { cn } from "#/lib/utils";

type CandidateBar = {
  source: string;
  value: number;
  percent: number;
  color: string;
};

const scoreStats = (decisions: TerminalDecisionRow[]) => {
  const scores = decisions
    .map((decision) => decision.scoreValue)
    .filter(Number.isFinite)
    .sort((left, right) => left - right);

  if (scores.length === 0) {
    return { median: 0, mad: 0, line: 0 };
  }

  const median = scores[Math.floor(scores.length / 2)] ?? 0;
  const deviations = scores
    .map((score) => Math.abs(score - median))
    .sort((left, right) => left - right);
  const mad = deviations[Math.floor(deviations.length / 2)] ?? 0;

  return { median, mad, line: median + mad };
};

const sourceBars = (
  decision: TerminalDecisionRow,
  kernels: TerminalKernel[],
): CandidateBar[] => {
  if (decision.signals.length > 0) {
    return [...decision.signals]
      .sort((left, right) => right.confidence - left.confidence)
      .slice(0, 3)
      .map((signal, index) => ({
        source: signal.source,
        value: signal.confidence,
        percent: clamp(signal.confidence * 100, 0, 100),
        color: index === 0 ? "var(--acc)" : "var(--info)",
      }));
  }

  const primary = kernels.find((kernel) => kernel.source === decision.source);
  const ranked = kernels
    .filter((kernel) => kernel.confidencePercent > 0)
    .sort((left, right) => right.confidencePercent - left.confidencePercent);
  const selected = [primary, ...ranked].filter(
    (kernel): kernel is TerminalKernel => kernel !== undefined,
  );
  const unique = selected.filter(
    (kernel, index, rows) =>
      rows.findIndex((row) => row.source === kernel.source) === index,
  );

  if (unique.length === 0) {
    return [
      {
        source: decision.source,
        value: decision.scoreValue,
        percent: clamp(decision.scoreValue * 100, 0, 100),
        color: "var(--acc)",
      },
    ];
  }

  return unique.slice(0, 3).map((kernel, index) => ({
    source: kernel.source,
    value: kernel.confidencePercent / 100,
    percent: kernel.confidencePercent,
    color: index === 0 ? "var(--acc)" : "var(--info)",
  }));
};

const verdictLabel = (verdict: TerminalDecisionRow["verdict"]) => {
  if (verdict === "allow") {
    return "allow";
  }

  if (verdict === "in-play") {
    return "in play";
  }

  return "below";
};

const verdictClass = (verdict: TerminalDecisionRow["verdict"]) => {
  if (verdict === "blocked") {
    return "bg-[rgba(213,120,106,0.16)] text-[var(--down)]";
  }

  if (verdict === "in-play") {
    return "bg-[rgba(232,163,61,0.16)] text-[var(--acc)]";
  }

  return "bg-[rgba(156,192,110,0.16)] text-[var(--up)]";
};

const DecisionFunnel = ({ model }: { model: TerminalModel }) => {
  const scanned =
    model.engine.candidates > 0 ? model.engine.candidates : model.decisions.length;
  const allowed = model.decisions.filter(
    (decision) => decision.verdict !== "blocked",
  ).length;
  const inPlay = model.decisions.filter(
    (decision) => decision.verdict === "in-play",
  ).length;
  const cards = [
    ["Scanned", scanned.toString(), "universe", "text-[var(--f1)]"],
    [
      "Quoted",
      model.engine.measurements.toString(),
      "fresh ticks",
      "text-[var(--info)]",
    ],
    ["In play", inPlay.toString(), ">= entry line", "text-[var(--acc)]"],
    ["Allowed", allowed.toString(), "edge clears", "text-[var(--up)]"],
  ];

  return (
    <div className="grid grid-cols-4 gap-2.5">
      {cards.map(([label, value, sub, color]) => (
        <div
          key={label}
          className="rounded-[4px] border border-[var(--line)] bg-[var(--surface)] px-3 py-3"
        >
          <div className="font-semibold text-[9.5px] text-[var(--f4)] uppercase tracking-[0.1em]">
            {label}
          </div>
          <div className={cn("mt-1 font-mono text-[26px] leading-none", color)}>
            {value}
          </div>
          <div className="mt-1 font-mono text-[9.5px] text-[var(--f4)]">
            {sub}
          </div>
        </div>
      ))}
    </div>
  );
};

const EntryLine = ({ model }: { model: TerminalModel }) => {
  const stats = scoreStats(model.decisions);

  return (
    <div className="flex items-center gap-3 rounded-[4px] border border-[var(--line)] bg-[var(--sunken)] px-3 py-2 font-mono text-[11.5px]">
      <span className="text-[var(--f3)]">entry line</span>
      <span className="font-semibold text-[var(--acc)]">{fixed(stats.line)}</span>
      <span className="text-[var(--f4)]">·</span>
      <span className="text-[var(--f3)]">median {fixed(stats.median)}</span>
      <span className="text-[var(--f4)]">·</span>
      <span className="text-[var(--f3)]">mad {fixed(stats.mad)}</span>
      <span className="ml-auto text-[var(--f4)]">
        support gate &gt;= 2 · edge &gt;= req
      </span>
    </div>
  );
};

const CandidateRow = ({
  decision,
  expanded,
  model,
}: {
  decision: TerminalDecisionRow;
  expanded: boolean;
  model: TerminalModel;
}) => {
  const stats = scoreStats(model.decisions);
  const bars = sourceBars(decision, model.kernels);
  const edge = decision.scoreValue - stats.line;
  const edgeColor = edge >= 0 ? "text-[var(--up)]" : "text-[var(--down)]";

  return (
    <div className="overflow-hidden rounded-[4px] border border-[var(--line)] bg-[var(--surface)]">
      <div className="grid grid-cols-[82px_1fr_152px_98px] items-center gap-3 px-3 py-2.5">
        <div>
          <div className="font-mono font-semibold text-[13px] text-[var(--f1)]">
            {decision.symbol}
          </div>
          <div className="font-mono text-[9px] text-[var(--f4)]">
            x{bars.length} src
          </div>
        </div>
        <div className="flex flex-col gap-1">
          {bars.map((bar) => (
            <div key={bar.source} className="flex items-center gap-2">
              <span className="w-16 font-mono text-[9px] text-[var(--f4)]">
                {bar.source}
              </span>
              <div className="h-1 flex-1 overflow-hidden rounded-sm bg-[var(--line)]">
                <div
                  className="h-full"
                  style={{
                    width: `${bar.percent}%`,
                    backgroundColor: bar.color,
                  }}
                />
              </div>
              <span className="w-9 text-right font-mono text-[9px] text-[var(--f3)]">
                {fixed(bar.value, 2)}
              </span>
            </div>
          ))}
        </div>
        <div>
          <div className="mb-1 flex justify-between font-mono text-[9.5px] text-[var(--f4)]">
            <span>combined</span>
            <span className="text-[var(--f1)]">{decision.scoreText}</span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-sm bg-[var(--line)]">
            <div
              className="h-full bg-[var(--info)]"
              style={{ width: `${clamp(decision.scoreValue * 100, 0, 100)}%` }}
            />
          </div>
          <div className={cn("mt-1 font-mono text-[9px]", edgeColor)}>
            edge {edge >= 0 ? "+" : "-"}
            {fixed(Math.abs(edge))} / {fixed(Math.abs(stats.line))}
          </div>
        </div>
        <div className="text-right">
          <span
            className={cn(
              "inline-block rounded-[2px] px-2 py-1 font-semibold text-[10px] uppercase",
              verdictClass(decision.verdict),
            )}
          >
            {verdictLabel(decision.verdict)}
          </span>
          <div className="mt-1 truncate font-mono text-[9px] text-[var(--f4)]">
            {decision.why}
          </div>
        </div>
      </div>
      {expanded ? (
        <div className="grid grid-cols-2 gap-5 border-[var(--line)] border-t bg-[var(--sunken)] px-4 py-3">
          <ScoreAttribution bars={bars} decision={decision} />
          <BackendProbePanel />
        </div>
      ) : null}
    </div>
  );
};

const ScoreAttribution = ({
  bars,
  decision,
}: {
  bars: CandidateBar[];
  decision: TerminalDecisionRow;
}) => (
  <div>
    <div className="mb-2 font-semibold text-[10px] text-[var(--f3)] uppercase tracking-[0.1em]">
      Score attribution
    </div>
    <div className="flex flex-col gap-1.5">
      {bars.map((bar) => {
        const delta = bar.value - decision.scoreValue;
        const left = delta >= 0 ? 50 : clamp(50 + delta * 50, 0, 50);
        const width = clamp(Math.abs(delta) * 100, 3, 50);

        return (
          <div key={bar.source} className="flex items-center gap-2">
            <span className="w-16 font-mono text-[9.5px] text-[var(--f4)]">
              {bar.source}
            </span>
            <div className="relative h-3 flex-1 rounded-sm bg-[var(--line)]">
              <div className="absolute top-0 bottom-0 left-1/2 w-px bg-[var(--f4)]" />
              <div
                className="absolute top-px bottom-px rounded-[1px]"
                style={{
                  left: `${left}%`,
                  width: `${width}%`,
                  backgroundColor: bar.color,
                }}
              />
            </div>
            <span className="w-12 text-right font-mono text-[9.5px] text-[var(--f3)]">
              {delta >= 0 ? "+" : "-"}
              {fixed(Math.abs(delta))}
            </span>
          </div>
        );
      })}
    </div>
  </div>
);

const BackendProbePanel = () => (
  <div>
    <div className="mb-2 font-semibold text-[10px] text-[var(--f3)] uppercase tracking-[0.1em]">
      Counterfactual probes · do(·)
    </div>
    <div className="rounded-[3px] border border-[var(--line)] bg-[var(--surface)] px-3 py-2 font-mono text-[10px] text-[var(--f4)]">
      backend counterfactual probes unavailable
    </div>
  </div>
);

const CandidateEvaluation = ({ model }: { model: TerminalModel }) => (
  <div>
    <div className="mb-2 flex items-baseline justify-between">
      <span className="font-semibold text-[10px] text-[var(--f3)] uppercase tracking-[0.13em]">
        Candidate evaluation
      </span>
      <span className="font-mono text-[9.5px] text-[var(--f4)]">
        backend decision trace
      </span>
    </div>
    <div className="flex flex-col gap-2">
      {model.decisions.length === 0 ? (
        <div className="rounded-[4px] border border-[var(--line)] bg-[var(--surface)] px-4 py-10 text-center font-mono text-[11px] text-[var(--f4)]">
          waiting for backend decision rows
        </div>
      ) : (
        model.decisions.map((decision, index) => (
          <CandidateRow
            key={decision.key}
            decision={decision}
            expanded={index === 0}
            model={model}
          />
        ))
      )}
    </div>
  </div>
);

export const DecisionTreeView = ({ model }: { model: TerminalModel }) => (
  <div className="grid h-full min-w-[1040px] grid-cols-[minmax(640px,1fr)_332px]">
    <div className="min-h-0 space-y-3 overflow-auto px-5 py-[18px]">
      <DecisionFunnel model={model} />
      <EntryLine model={model} />
      <CandidateEvaluation model={model} />
    </div>
    <DecisionSideRail model={model} />
  </div>
);
