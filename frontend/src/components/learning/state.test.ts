import { describe, expect, it } from "vitest";
import { Builder } from "flatbuffers";
import { learningStore, receiveLearning } from "#/collections/learning";
import { learningFixture } from "./fixture";
import { projectLearning } from "./state";

describe("projectLearning", () => {
 it("decodes real FlatBuffers into the same signed reading for the dashboard and toolbar", () => {
  const source = learningFixture();
  const builder = new Builder();
  builder.finish(source.pack(builder));
  receiveLearning(builder.asUint8Array());
  const view = projectLearning(learningStore.state!, "");
  expect(view.skill.mean).toBe(-.005);
  expect(view.skill.defined).toBe(true);
  expect(view.lanes?.[0].profit).toBe(-2);
  expect(view.desk?.traders[0].wealth).toBe(-.01);
  expect(view.resolved).toBe(3);
  expect(view.lanes?.[0].unresolved).toBe(17);
 });
 it("does not turn missing evidence into a measured zero", () => {
  const source = learningFixture();
  source.agents[0].reading = null;
  const view = projectLearning(source, "");
  expect(view.skill.defined).toBe(false);
 });
});
