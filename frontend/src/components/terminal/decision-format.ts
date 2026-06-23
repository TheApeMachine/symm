export const clamp = (value: number, min: number, max: number): number =>
  Math.min(max, Math.max(min, value));

export const fixed = (value: number, digits = 3): string => {
  if (!Number.isFinite(value)) {
    return "0";
  }

  return value.toFixed(digits);
};
