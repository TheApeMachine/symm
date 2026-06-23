import type { TerminalKernel, TerminalModel } from "#/components/terminal/model";
import { clamp, fixed } from "#/components/terminal/decision-format";

const kernelValue = (
  kernels: TerminalKernel[],
  source: string,
): { value: number; available: boolean } => {
  const kernel = kernels.find((row) => row.source === source);

  if (kernel === undefined || kernel.confidencePercent <= 0) {
    return { value: 0, available: false };
  }

  return { value: kernel.confidencePercent / 100, available: true };
};

const CausalLadder = ({ model }: { model: TerminalModel }) => {
  const rows = [
    {
      name: "Association",
      desc: "P(y | x) · correlation",
      reading: kernelValue(model.kernels, "correlation"),
      color: "var(--info)",
    },
    {
      name: "Intervention",
      desc: "P(y | do(x)) · acting",
      reading: kernelValue(model.kernels, "causal"),
      color: "var(--acc)",
    },
    {
      name: "Counterfactual",
      desc: "P(y_x | x',y')",
      reading: {
        value: model.cognitive?.lookaheadScore ?? 0,
        available: model.cognitive !== null,
      },
      color: "var(--up)",
    },
  ];

  return (
    <div className="rounded-[4px] border border-[var(--line)] bg-[var(--sunken)] p-3">
      <div className="font-semibold text-[12px] text-[var(--f1)]">
        Causal ladder
      </div>
      <div className="mt-1 mb-3 font-mono text-[9.5px] text-[var(--f4)]">
        pearl do-calculus · {model.cognitive?.regimePrefix || "waiting"}
      </div>
      <div className="flex flex-col gap-2">
        {rows.map((row, index) => (
          <div
            key={row.name}
            className="rounded-[3px] border border-[var(--line)] bg-[var(--surface)] px-2.5 py-2"
          >
            <div className="flex justify-between">
              <span className="font-semibold text-[11.5px] text-[var(--f1)]">
                {index + 1}. {row.name}
              </span>
              <span
                className="font-mono text-[11px]"
                style={{ color: row.color }}
              >
                {row.reading.available ? fixed(row.reading.value) : "waiting"}
              </span>
            </div>
            <div className="my-1 font-mono text-[9px] text-[var(--f4)]">
              {row.desc}
            </div>
            <div className="h-1.5 overflow-hidden rounded-sm bg-[var(--line)]">
              <div
                className="h-full"
                style={{
                  width: `${clamp(row.reading.value * 100, 0, 100)}%`,
                  backgroundColor: row.color,
                }}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

const CognitiveBeam = ({ model }: { model: TerminalModel }) => {
  const reading = model.cognitive;
  const entropyThreshold = reading?.entropyThreshold ?? 0;
  const entropyPercent =
    entropyThreshold > 0
      ? clamp(((reading?.entropyBits ?? 0) / entropyThreshold) * 100, 0, 100)
      : 0;
  const meters = [
    {
      label: "Entropy gate",
      value:
        reading !== null
          ? `${fixed(reading.entropyBits, 2)} / ${fixed(entropyThreshold, 1)} bits`
          : "waiting",
      percent: entropyPercent,
      color: "var(--up)",
    },
    {
      label: "Class confidence",
      value:
        reading !== null ? `${Math.round(reading.classConfidence * 100)}%` : "0%",
      percent: clamp((reading?.classConfidence ?? 0) * 100, 0, 100),
      color: "var(--info)",
    },
    {
      label: "Lookahead beam",
      value: reading !== null ? fixed(reading.lookaheadScore) : "waiting",
      percent: clamp((reading?.lookaheadScore ?? 0) * 100, 0, 100),
      color: "var(--acc)",
    },
  ];

  return (
    <div className="rounded-[4px] border border-[var(--line)] bg-[var(--sunken)] p-3">
      <div className="flex items-center justify-between">
        <span className="font-semibold text-[12px] text-[var(--f1)]">
          Cognitive beam
        </span>
        <span className="rounded-full border border-[var(--line2)] px-2 py-px font-mono text-[9px] text-[var(--info)]">
          cohort {reading?.regimeCohort ?? 0}
        </span>
      </div>
      <div className="mt-2 font-mono text-[9.5px] text-[var(--f4)]">
        DMT sequence
      </div>
      <div className="mt-1 break-all rounded-[3px] border border-[var(--line)] bg-[var(--bg)] p-2 font-mono text-[10px] text-[var(--f2)]">
        {reading?.sequence || "waiting for DMT sequence"}
      </div>
      <div className="mt-3 flex flex-col gap-2">
        {meters.map((meter) => (
          <div key={meter.label}>
            <div className="mb-1 flex justify-between text-[10.5px]">
              <span className="text-[var(--f3)]">{meter.label}</span>
              <span className="font-mono text-[var(--f1)]">{meter.value}</span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-sm bg-[var(--line)]">
              <div
                className="h-full"
                style={{
                  width: `${meter.percent}%`,
                  backgroundColor: meter.color,
                }}
              />
            </div>
          </div>
        ))}
      </div>
      <div className="mt-3 grid grid-cols-2 gap-2 font-mono text-[10px]">
        <div className="flex justify-between">
          <span className="text-[var(--f4)]">winner</span>
          <span className="text-[var(--acc)]">
            {reading?.winnerClass || "pending"}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-[var(--f4)]">paths</span>
          <span className="text-[var(--f1)]">{reading?.lookaheadPaths ?? 0}</span>
        </div>
      </div>
    </div>
  );
};

export const DecisionSideRail = ({ model }: { model: TerminalModel }) => (
  <div className="min-h-0 space-y-3 overflow-auto border-[var(--line)] border-l bg-[var(--surface)] p-3.5">
    <CausalLadder model={model} />
    <CognitiveBeam model={model} />
  </div>
);
