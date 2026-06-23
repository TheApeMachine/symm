import type { TerminalModel } from "#/components/terminal/model";
import { toneClasses } from "#/components/terminal/tone";
import { cn } from "#/lib/utils";

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

const Stat = ({
  value,
  label,
  accent = false,
}: {
  value: string;
  label: string;
  accent?: boolean;
}) => (
  <div>
    <div
      className={cn(
        "font-mono text-2xl leading-none",
        accent ? "text-amber-300" : "text-stone-100",
      )}
    >
      {value}
    </div>
    <div className="mt-1 text-[9px] text-stone-600">{label}</div>
  </div>
);

export const HealthPanel = ({ model }: { model: TerminalModel }) => (
  <div className="rounded border border-stone-800 bg-black/25 p-3">
    <div className="flex items-center justify-between">
      <span className="font-semibold text-stone-100 text-xs">
        System health
      </span>
      <span
        className={cn(
          "rounded-full border px-2 py-0.5 font-semibold text-[10px] uppercase",
          toneClasses(model.health.degraded > 0 ? "bad" : "good"),
        )}
      >
        {model.health.label}
      </span>
    </div>
    <div className="mt-3 grid grid-cols-3 gap-3">
      <Stat
        value={`${model.health.healthy}/${model.health.total}`}
        label="healthy"
      />
      <Stat value={`${model.health.averageConfidence}%`} label="avg conf" />
      <Stat value={model.health.firing.toString()} label="firing" accent />
    </div>
    <div className="mt-3 space-y-2">
      <Meter
        label="Healthy"
        value={model.health.healthy.toString()}
        percent={(model.health.healthy / Math.max(model.health.total, 1)) * 100}
        color="bg-emerald-300"
      />
      <Meter
        label="Warming"
        value={model.health.warming.toString()}
        percent={(model.health.warming / Math.max(model.health.total, 1)) * 100}
        color="bg-amber-300"
      />
      <Meter
        label="Degraded"
        value={model.health.degraded.toString()}
        percent={
          (model.health.degraded / Math.max(model.health.total, 1)) * 100
        }
        color="bg-rose-300"
      />
    </div>
  </div>
);

export const RadarPanel = ({ model }: { model: TerminalModel }) => {
  const values = [
    model.health.averageConfidence / 100,
    model.engine.signalsPercent / 100,
    model.health.healthy / Math.max(model.health.total, 1),
    model.health.firing / Math.max(model.health.total, 1),
    model.positions.length / Math.max(model.engine.open, 1),
  ];
  const units = [
    [0, -1],
    [0.951, -0.309],
    [0.588, 0.809],
    [-0.588, 0.809],
    [-0.951, -0.309],
  ];
  const points = units
    .map(
      ([x, y], index) =>
        `${110 + x * 84 * values[index]},${105 + y * 84 * values[index]}`,
    )
    .join(" ");

  return (
    <div className="rounded border border-stone-800 bg-black/25 p-3">
      <div className="mb-2 font-semibold text-stone-100 text-xs">
        Regime radar
      </div>
      <svg viewBox="0 0 220 210" className="block w-full">
        <polygon
          points="110,21 190,79 159,173 61,173 30,79"
          fill="none"
          stroke="#3a342b"
        />
        <polygon
          points="110,49 163,87 142,154 78,154 57,87"
          fill="none"
          stroke="#2b251e"
        />
        <polygon
          points={points}
          fill="rgba(232,163,61,0.22)"
          stroke="#e8a33d"
          strokeWidth="1.6"
        />
        {["conf", "signal", "health", "fire", "open"].map((label, index) => (
          <text
            key={label}
            x={110 + units[index][0] * 98}
            y={105 + units[index][1] * 98}
            textAnchor="middle"
            fontSize="9"
            fill="#938a7e"
          >
            {label}
          </text>
        ))}
      </svg>
    </div>
  );
};
