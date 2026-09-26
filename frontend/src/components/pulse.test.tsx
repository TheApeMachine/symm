import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Pulse } from "#/components/pulse";

describe("Pulse", () => {
 it("keeps absent native outputs absent", () => {
  const html = renderToStaticMarkup(<Pulse />);
  expect(html).toContain('data-read="tick"');
  expect(html).toContain('data-read="phase"');
  expect(html).not.toContain('meas');
 });
 it("renders exact graph observations and zero decisions", () => {
  const html = renderToStaticMarkup(<Pulse observations="9007199254740993" phase="paper" decisions="0" open="3" />);
  expect(html).toContain("9007199254740993");
  expect(html).toContain("paper");
  expect(html).toContain('data-read="cand"');
  expect(html).toContain('>0</span>');
 });
});
