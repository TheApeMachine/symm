import { XIcon } from "lucide-react";
import type {
  TerminalKernel,
  TerminalModel,
} from "#/components/terminal/model";
import { toneClasses } from "#/components/terminal/tone";
import { Button } from "#/components/ui/button";
import { cn } from "#/lib/utils";

const kernelTone = (kernel: TerminalKernel) => {
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

const SIGNAL_COPY: Record<
  string,
  {
    sub: string;
    blurb: string;
  }
> = {
  fluid: {
    sub: "fluid · vol-rank × Δ",
    blurb:
      "Navier-Stokes pressure field over the market cross-section. Whale carriers bend the density surface; turbulence flags regime breaks before price confirms.",
  },
  pumpdump: {
    sub: "pumpdump · ignition",
    blurb:
      "Pumpdump measurement emitted by the backend from raw market artifacts and projected directly into the terminal signal surface.",
  },
  hawkes: {
    sub: "hawkes · excitation",
    blurb:
      "Self-excitation pressure from recent event flow. The backend owns the sample and the terminal only renders the emitted measurement.",
  },
  causal: {
    sub: "causal · ladder",
    blurb:
      "Causal branch pressure from backend measurements and candidate decision traces.",
  },
  manifold: {
    sub: "manifold · latent",
    blurb: "Latent geometry emitted by the backend for the current artifact stream.",
  },
  correlation: {
    sub: "correlation · cross-section",
    blurb: "Cross-symbol correlation pressure from backend measurement artifacts.",
  },
};

const kernelCopy = (kernel: TerminalKernel) =>
  SIGNAL_COPY[kernel.source] ?? {
    sub: kernel.category,
    blurb:
      "Backend measurement emitted from raw artifacts and projected into the terminal signal surface.",
  };

export const KernelInspector = ({
  kernel,
  onClose,
  onInsight,
}: {
  kernel: TerminalKernel | null;
  onClose: () => void;
  onInsight: () => void;
}) => {
  if (kernel === null) {
    return null;
  }

  const tone = kernelTone(kernel);

  return (
    <div
      className="absolute inset-y-0 left-[264px] right-[350px] z-20 flex items-start justify-center bg-black/55 p-8 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md overflow-hidden rounded-md border border-stone-700 bg-[#17140f] shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-3 border-stone-800 border-b px-4 py-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="truncate font-serif text-stone-100 text-xl">
                {kernel.name}
              </h2>
              <span
                className={cn(
                  "rounded-sm border px-1.5 py-0.5 font-semibold text-[9px] uppercase",
                  toneClasses(tone),
                )}
              >
                {kernel.statusLabel}
              </span>
            </div>
            <div className="mt-1 font-mono text-[10px] text-stone-600">
              {kernel.category}
            </div>
          </div>
          <Button size="icon-xs" variant="ghost" onClick={onClose}>
            <XIcon />
          </Button>
        </div>
        <KernelMeters kernel={kernel} />
        <div className="flex items-center justify-between gap-3 border-stone-800 border-t bg-black/25 px-4 py-3">
          <div className="font-mono text-[10px] text-stone-600">
            <div>active {kernel.activeText}</div>
            <div>
              {kernel.observedText} · {kernel.samplesText}
            </div>
          </div>
          <Button size="sm" variant="outline" onClick={onInsight}>
            Open in signal insight
          </Button>
        </div>
      </div>
    </div>
  );
};

const KernelMeters = ({ kernel }: { kernel: TerminalKernel }) => (
  <div className="space-y-3 px-4 py-4">
    <Meter
      label="Confidence"
      value={kernel.confidenceText}
      percent={kernel.confidencePercent}
    />
    <Meter
      label="Surprise"
      value={kernel.surpriseText}
      percent={kernel.surprisePercent}
      color="bg-amber-300"
    />
    <Meter
      label="Health"
      value={`${kernel.healthPercent}%`}
      percent={kernel.healthPercent}
      color="bg-emerald-300"
    />
    <Meter
      label="Strength"
      value={kernel.strengthText}
      percent={kernel.confidencePercent}
      color="bg-cyan-300"
    />
    {kernel.faultText ? (
      <div className="font-mono text-[10px] text-rose-300">
        {kernel.faultText}
      </div>
    ) : null}
  </div>
);

const Meter = ({
  label,
  value,
  percent,
  color = "bg-cyan-300",
}: {
  label: string;
  value: string;
  percent: number;
  color?: string;
}) => (
  <div>
    <div className="mb-1 flex justify-between font-mono text-[10px]">
      <span className="text-stone-500">{label}</span>
      <span className="text-stone-200">{value}</span>
    </div>
    <div className="h-1.5 overflow-hidden rounded-sm bg-stone-800">
      <div className={cn("h-full", color)} style={{ width: `${percent}%` }} />
    </div>
  </div>
);

export const SignalDetail = ({ kernel }: { kernel: TerminalKernel | null }) => {
  if (kernel === null) {
    return (
      <div className="p-5 font-mono text-stone-600 text-xs">
        Waiting for signal readings
      </div>
    );
  }

  const copy = kernelCopy(kernel);

  return (
    <div className="min-h-0 overflow-auto p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="font-serif text-2xl text-[var(--f1)]">
            {kernel.name}
          </h2>
          <div className="mt-1 font-mono text-[11px] text-[var(--f4)]">
            {copy.sub}
          </div>
        </div>
        <span
          className={cn(
            "rounded-sm border px-2 py-1 font-semibold text-[10px] uppercase",
            toneClasses(kernelTone(kernel)),
          )}
        >
          {kernel.statusLabel}
        </span>
      </div>
      <p className="mt-4 max-w-2xl font-serif text-[15px] text-[var(--f2)] leading-relaxed">
        {copy.blurb}
      </p>
      <div className="mt-5 grid grid-cols-2 gap-x-6 gap-y-4">
        <KernelMeters kernel={kernel} />
      </div>
    </div>
  );
};

export const AllocationView = ({ model }: { model: TerminalModel }) => (
  <div className="min-h-0 overflow-auto p-4">
    <div className="mb-3 flex items-center gap-4 font-mono text-[11px] text-stone-500">
      <span>cross-section</span>
      <span>decisions {model.decisions.length}</span>
      <span>open {model.positions.length}</span>
      <span className="ml-auto">backend values only</span>
    </div>
    <div className="flex flex-col border-stone-800 border-t">
      {model.decisions.map((decision) => {
        const scorePercent = Math.max(
          0,
          Math.min(100, decision.scoreValue * 100),
        );

        return (
          <div
            key={decision.key}
            className="grid grid-cols-[80px_1fr_70px_86px] items-center gap-3 border-stone-800 border-b py-2"
          >
            <span className="font-mono font-semibold text-stone-100 text-xs">
              {decision.symbol}
            </span>
            <div className="relative h-5">
              <div className="absolute top-2 right-0 left-0 h-px bg-stone-800" />
              <div
                className="absolute top-[7px] h-1 rounded-sm bg-amber-300"
                style={{
                  left: "50%",
                  width: `${Math.max(0, scorePercent - 50)}%`,
                }}
              />
              <div
                className="absolute top-1 size-3 rounded-full bg-cyan-300"
                style={{ left: `${scorePercent}%` }}
              />
            </div>
            <span className="text-right font-mono text-[10px] text-stone-300">
              {decision.scoreText}
            </span>
            <span
              className={cn(
                "rounded-sm border px-1.5 py-0.5 text-center font-semibold text-[9px] uppercase",
                toneClasses(decision.verdict === "blocked" ? "bad" : "good"),
              )}
            >
              {decision.verdict}
            </span>
          </div>
        );
      })}
    </div>
  </div>
);
