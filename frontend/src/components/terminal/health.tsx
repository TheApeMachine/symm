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
      <span className="text-[var(--f4)]">{label}</span>
      <span className="text-[var(--f2)]">{value}</span>
    </div>
    <div className="h-1.5 overflow-hidden rounded-sm bg-[var(--line)]">
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
        accent ? "text-[var(--acc)]" : "text-[var(--f1)]",
      )}
    >
      {value}
    </div>
    <div className="mt-1 text-[9px] text-[var(--f4)]">{label}</div>
  </div>
);

export const HealthPanel = ({ model }: { model: TerminalModel }) => (
  <div className="rounded-[3px] border border-[var(--line)] bg-[var(--sunken)] p-3">
    <div className="flex items-center justify-between">
      <span className="font-semibold text-[var(--f1)] text-xs">
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
        color="bg-[var(--up)]"
      />
      <Meter
        label="Warming"
        value={model.health.warming.toString()}
        percent={(model.health.warming / Math.max(model.health.total, 1)) * 100}
        color="bg-[var(--acc)]"
      />
      <Meter
        label="Degraded"
        value={model.health.degraded.toString()}
        percent={
          (model.health.degraded / Math.max(model.health.total, 1)) * 100
        }
        color="bg-[var(--down)]"
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
    <div className="rounded-[3px] border border-[var(--line)] bg-[var(--sunken)] p-3">
      <div className="mb-2 font-semibold text-[var(--f1)] text-xs">
        Regime radar
      </div>
      <div className="mb-2 font-mono text-[9.5px] text-[var(--f4)]">
        cross-section mean · market
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
          points="110,77 137,94 126,134 94,134 83,94"
          fill="none"
          stroke="#2b251e"
        />
        {units.map(([x, y]) => (
          <line
            key={`${x}:${y}`}
            x1="110"
            y1="105"
            x2={110 + x * 84}
            y2={105 + y * 84}
            stroke="#2b251e"
          />
        ))}
        <polygon
          points={points}
          fill="rgba(232,163,61,0.22)"
          stroke="#e8a33d"
          strokeWidth="1.6"
        />
        {["volatility", "trend", "bullish", "bearish", "chop"].map(
          (label, index) => (
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
          ),
        )}
      </svg>
    </div>
  );
};
