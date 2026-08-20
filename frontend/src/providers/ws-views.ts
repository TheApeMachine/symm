import * as flatbuffers from "flatbuffers";
import type { JSONSerializable } from "#/components/ui/paint";
import { Batch } from "#/providers/telemetry/telemetry/batch";
import { FrameEntry } from "#/providers/telemetry/telemetry/frame-entry";
import { Frame, unionToFrame } from "#/providers/telemetry/telemetry/frame";
import { Measurement } from "#/providers/telemetry/telemetry/measurement";
import { MeasurementsFrame } from "#/providers/telemetry/telemetry/measurements-frame";
import { Metric } from "#/providers/telemetry/telemetry/metric";
import { NamedNumber } from "#/providers/telemetry/telemetry/named-number";
import { decodeTelemetryTable } from "./ws-flatbuffers";

export type TelemetryBatchView = {
  byteBuffer: flatbuffers.ByteBuffer;
  batch: Batch;
};

export type TelemetryFrameView = {
  batch: TelemetryBatchView;
  index: number;
  type: Frame;
};

export type MeasurementView = {
  frame: TelemetryFrameView;
  row: number;
};

const entryAt = (view: TelemetryFrameView, entry = new FrameEntry()) => {
  const result = view.batch.batch.frames(view.index, entry);

  if (result === null || result.frameType() !== view.type) {
    throw new Error(`telemetry batch frame ${view.index} is missing`);
  }

  return result;
};

export const openTelemetryBatch = (buffer: ArrayBuffer) => {
  const byteBuffer = new flatbuffers.ByteBuffer(new Uint8Array(buffer));

  if (!byteBuffer.__has_identifier("SYMB")) {
    throw new Error("telemetry batch has no SYMB FlatBuffers identifier");
  }

  const batch = Batch.getRootAsBatch(byteBuffer);

  if (batch.framesLength() === 0) {
    throw new Error("telemetry batch is truncated");
  }

  const batchView = { byteBuffer, batch };
  const frames = new Array<TelemetryFrameView>(batch.framesLength());
  const entry = new FrameEntry();

  for (let index = 0; index < frames.length; index += 1) {
    const current = batch.frames(index, entry);

    if (current === null || current.frameType() === Frame.NONE) {
      throw new Error(`telemetry batch frame ${index} has no payload`);
    }

    frames[index] = { batch: batchView, index, type: current.frameType() };
  }

  return frames;
};

export const measurementsTable = (
  view: TelemetryFrameView,
  table = new MeasurementsFrame(),
) => {
  if (view.type !== Frame.MeasurementsFrame) {
    throw new Error("telemetry frame is not a measurements frame");
  }

  const frame = entryAt(view).frame(table);

  if (frame === null) {
    throw new Error("measurements frame has no table");
  }

  return frame as MeasurementsFrame;
};

export const materializeFrameView = (
  view: TelemetryFrameView,
): Record<string, JSONSerializable> => {
  const entry = entryAt(view);
  const table = unionToFrame(view.type, entry.frame.bind(entry));

  if (table === null) {
    throw new Error(`telemetry batch frame ${view.index} has no table`);
  }

  return decodeTelemetryTable(view.type, table);
};

const instant = (nanoseconds: bigint) =>
  nanoseconds === 0n
    ? undefined
    : new Date(Number(nanoseconds / 1_000_000n)).toISOString();

export const materializeMeasurement = (
  view: MeasurementView,
): JSONSerializable => {
  const table = measurementsTable(view.frame);
  const row = table.rows(view.row, new Measurement());

  if (row === null) {
    throw new Error(`measurement row ${view.row} is missing`);
  }

  const metrics: Record<string, JSONSerializable> = {};
  const metric = new Metric();

  for (let index = 0; index < row.metricsLength(); index += 1) {
    const current = row.metrics(index, metric);
    const name = current?.name();

    if (current === null || name == null) {
      throw new Error(`measurement metric ${index} is missing`);
    }

    metrics[name] = {
      raw: current.raw(),
      ...(current.hasNormalized() ? { normalized: current.normalized() } : {}),
      ...(current.unit() ? { unit: current.unit()! } : {}),
    };
  }

  const metadata: Record<string, JSONSerializable> = {};
  const named = new NamedNumber();

  for (let index = 0; index < row.metadataLength(); index += 1) {
    const current = row.metadata(index, named);
    const name = current?.name();

    if (current === null || name == null) {
      throw new Error(`measurement metadata ${index} is missing`);
    }

    metadata[name] = current.value();
  }

  const at = instant(row.at());
  const observedFrom = instant(row.observedFrom());
  const peerAt = instant(row.peerAt());
  const peerObservedFrom = instant(row.peerObservedFrom());

  return {
    ...(row.id() ? { id: row.id()! } : {}),
    source: row.source()!,
    symbol: row.symbol()!,
    tick: Number(row.tick()),
    ...(row.peer() ? { peer: row.peer()! } : {}),
    ...(at ? { at } : {}),
    ...(observedFrom ? { observedFrom } : {}),
    horizon: Number(row.horizon()),
    ...(peerAt ? { peerAt } : {}),
    ...(peerObservedFrom ? { peerObservedFrom } : {}),
    maturity: row.maturity(),
    metrics,
    metadata,
  };
};
