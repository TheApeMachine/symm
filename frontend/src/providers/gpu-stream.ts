import type { StreamMetric, StreamRegistration } from "./stream-protocol";
import { Measurement } from "#/providers/telemetry/telemetry/measurement";
import { Metric } from "#/providers/telemetry/telemetry/metric";
import {
  chartFragmentShader,
  chartVertexShader,
  reductionShader,
  vertexShader,
} from "./gpu-stream-shaders";

type TextureLevel = {
  width: number;
  values: WebGLTexture;
  metadata: WebGLTexture;
  framebuffer: WebGLFramebuffer;
};

const compile = (gl: WebGL2RenderingContext, type: number, source: string) => {
  const shader = gl.createShader(type);

  if (shader === null) {
    throw new Error("stream renderer could not allocate a shader");
  }

  gl.shaderSource(shader, source);
  gl.compileShader(shader);

  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    const message =
      gl.getShaderInfoLog(shader) ?? "stream shader compilation failed";
    gl.deleteShader(shader);
    throw new Error(message);
  }

  return shader;
};

const program = (
  gl: WebGL2RenderingContext,
  vertex: string,
  fragment: string,
) => {
  const result = gl.createProgram();

  if (result === null) {
    throw new Error("stream renderer could not allocate a program");
  }

  const vertexShader = compile(gl, gl.VERTEX_SHADER, vertex);
  const fragmentShader = compile(gl, gl.FRAGMENT_SHADER, fragment);
  gl.attachShader(result, vertexShader);
  gl.attachShader(result, fragmentShader);
  gl.linkProgram(result);
  gl.deleteShader(vertexShader);
  gl.deleteShader(fragmentShader);

  if (!gl.getProgramParameter(result, gl.LINK_STATUS)) {
    const message =
      gl.getProgramInfoLog(result) ?? "stream program link failed";
    gl.deleteProgram(result);
    throw new Error(message);
  }

  return result;
};

const texture = (gl: WebGL2RenderingContext, width: number) => {
  const result = gl.createTexture();

  if (result === null) {
    throw new Error("stream renderer could not allocate a texture");
  }

  gl.bindTexture(gl.TEXTURE_2D, result);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texStorage2D(gl.TEXTURE_2D, 1, gl.RGBA32F, width, 1);
  return result;
};

const capacityFor = (required: number) => {
  let capacity = 1;

  while (capacity < required) {
    capacity *= 2;
  }

  return capacity;
};

export const temporalLevel = (count: number, pixels: number) => {
  let level = 0;
  let buckets = count;

  while (buckets > pixels) {
    buckets = Math.ceil(buckets / 2);
    level += 1;
  }

  return level;
};

/*
GPUStreamRenderer owns a circular level-zero texture and a min/max/first/last
reduction pyramid. Network ingestion only writes one texel; rendering selects
the first level bounded by the physical viewport width.
*/
export class GPUStreamRenderer {
  private readonly gl: WebGL2RenderingContext;
  private readonly capacity: number;
  private readonly levels: TextureLevel[] = [];
  private readonly reduceProgram: WebGLProgram;
  private readonly chartProgram: WebGLProgram;
  private readonly vertexArray: WebGLVertexArrayObject;
  private readonly uniformLocations = new WeakMap<
    WebGLProgram,
    Map<string, WebGLUniformLocation>
  >();
  private readonly sampleValues = new Float32Array(4);
  private readonly sampleMetadata = new Float32Array(4);
  private readonly metricView = new Metric();
  private head = 0;
  private count = 0;
  private origin: bigint | null = null;
  private frame = 0;
  private metric: StreamMetric;

  constructor(private readonly registration: StreamRegistration) {
    const gl = registration.canvas.getContext("webgl2", {
      alpha: false,
      antialias: false,
    }) as WebGL2RenderingContext | null;

    if (gl === null || gl.getExtension("EXT_color_buffer_float") === null) {
      throw new Error("stream renderer requires WebGL2 float render targets");
    }

    this.gl = gl;
    this.metric = registration.metric;
    this.capacity = capacityFor(registration.capacity);
    this.reduceProgram = program(gl, vertexShader, reductionShader);
    this.chartProgram = program(gl, chartVertexShader, chartFragmentShader);
    const vertexArray = gl.createVertexArray();

    if (vertexArray === null) {
      throw new Error("stream renderer could not allocate a vertex array");
    }

    this.vertexArray = vertexArray;
    gl.bindVertexArray(vertexArray);
    this.createLevels();
    this.resize(
      registration.width,
      registration.height,
      registration.pixelRatio,
    );
  }

  updateMetric(metric: StreamMetric) {
    this.metric = metric;
    this.head = 0;
    this.count = 0;
    this.origin = null;
    this.schedule();
  }

  matches(source: string, symbol: string) {
    return source === this.metric.source && symbol === this.metric.symbol;
  }

  ingest(row: Measurement) {
    let value: number | undefined;
    let baseline = Number.NaN;
    let decay = Number.NaN;

    for (let index = 0; index < row.metricsLength(); index += 1) {
      const current = row.metrics(index, this.metricView);

      if (current === null) {
        throw new Error(`measurement metric ${index} is missing`);
      }

      const name = current.name();

      if (name === this.metric.value) {
        value = current.raw();
      }

      if (name === this.metric.baseline) {
        baseline = current.raw();
      }

      if (name === this.metric.decay) {
        decay = current.raw();
      }
    }

    if (value === undefined) {
      return;
    }

    const at = row.at();
    this.origin ??= at;
    const time = Number(at - this.origin) / 1e9;
    this.sampleValues.fill(value);
    this.sampleMetadata[0] = time;
    this.sampleMetadata[1] = time;
    this.sampleMetadata[2] = baseline;
    this.sampleMetadata[3] = decay;
    this.write(this.levels[0]!.values, this.sampleValues);
    this.write(this.levels[0]!.metadata, this.sampleMetadata);
    this.head = (this.head + 1) & (this.capacity - 1);
    this.count = Math.min(this.count + 1, this.capacity);
    this.schedule();
  }

  resize(width: number, height: number, pixelRatio: number) {
    this.registration.canvas.width = Math.floor(width * pixelRatio);
    this.registration.canvas.height = Math.floor(height * pixelRatio);
    this.registration.width = width;
    this.registration.height = height;
    this.registration.pixelRatio = pixelRatio;
    this.schedule();
  }

  dispose() {
    cancelAnimationFrame(this.frame);

    for (const level of this.levels) {
      this.gl.deleteTexture(level.values);
      this.gl.deleteTexture(level.metadata);
      this.gl.deleteFramebuffer(level.framebuffer);
    }

    this.gl.deleteProgram(this.reduceProgram);
    this.gl.deleteProgram(this.chartProgram);
    this.gl.deleteVertexArray(this.vertexArray);
  }

  private createLevels() {
    let width = this.capacity;

    while (true) {
      const framebuffer = this.gl.createFramebuffer();

      if (framebuffer === null) {
        throw new Error("stream renderer could not allocate a framebuffer");
      }

      const level = {
        width,
        values: texture(this.gl, width),
        metadata: texture(this.gl, width),
        framebuffer,
      };
      this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, framebuffer);
      this.gl.framebufferTexture2D(
        this.gl.FRAMEBUFFER,
        this.gl.COLOR_ATTACHMENT0,
        this.gl.TEXTURE_2D,
        level.values,
        0,
      );
      this.gl.framebufferTexture2D(
        this.gl.FRAMEBUFFER,
        this.gl.COLOR_ATTACHMENT1,
        this.gl.TEXTURE_2D,
        level.metadata,
        0,
      );
      this.gl.drawBuffers([
        this.gl.COLOR_ATTACHMENT0,
        this.gl.COLOR_ATTACHMENT1,
      ]);

      if (
        this.gl.checkFramebufferStatus(this.gl.FRAMEBUFFER) !==
        this.gl.FRAMEBUFFER_COMPLETE
      ) {
        throw new Error("stream renderer framebuffer is incomplete");
      }

      this.levels.push(level);

      if (width === 1) {
        return;
      }

      width /= 2;
    }
  }

  private write(target: WebGLTexture, values: Float32Array) {
    this.gl.bindTexture(this.gl.TEXTURE_2D, target);
    this.gl.texSubImage2D(
      this.gl.TEXTURE_2D,
      0,
      this.head,
      0,
      1,
      1,
      this.gl.RGBA,
      this.gl.FLOAT,
      values,
    );
  }

  private schedule() {
    if (this.frame === 0) {
      this.frame = requestAnimationFrame(this.render);
    }
  }

  private readonly render = () => {
    this.frame = 0;
    const gl = this.gl;
    gl.bindVertexArray(this.vertexArray);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    gl.viewport(
      0,
      0,
      this.registration.canvas.width,
      this.registration.canvas.height,
    );
    gl.clearColor(...this.registration.palette.background);
    gl.clear(gl.COLOR_BUFFER_BIT);

    if (this.count === 0) {
      return;
    }

    this.reduce();
    this.draw();
  };

  private reduce() {
    const gl = this.gl;
    gl.useProgram(this.reduceProgram);
    let sourceCount = this.count;

    for (
      let index = 1;
      index < this.levels.length && sourceCount > 1;
      index += 1
    ) {
      const source = this.levels[index - 1]!;
      const target = this.levels[index]!;
      const outputCount = Math.ceil(sourceCount / 2);
      gl.bindFramebuffer(gl.FRAMEBUFFER, target.framebuffer);
      gl.viewport(0, 0, outputCount, 1);
      this.bindTexture(this.reduceProgram, "uValues", source.values, 0);
      this.bindTexture(this.reduceProgram, "uMetadata", source.metadata, 1);
      this.uniformInt(this.reduceProgram, "uSourceWidth", source.width);
      this.uniformInt(this.reduceProgram, "uSourceCount", sourceCount);
      this.uniformInt(
        this.reduceProgram,
        "uOldest",
        index === 1 ? this.oldest() : 0,
      );
      this.uniformInt(this.reduceProgram, "uRing", index === 1 ? 1 : 0);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
      sourceCount = outputCount;
    }
  }

  private draw() {
    const gl = this.gl;
    const drawLevelIndex = temporalLevel(
      this.count,
      this.registration.canvas.width,
    );
    const drawLevel = this.levels[drawLevelIndex]!;
    const drawCount = Math.ceil(this.count / 2 ** drawLevelIndex);
    const scaleLevelIndex = Math.ceil(Math.log2(this.count));
    const scaleLevel = this.levels[scaleLevelIndex]!;
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    gl.viewport(
      0,
      0,
      this.registration.canvas.width,
      this.registration.canvas.height,
    );
    gl.useProgram(this.chartProgram);
    this.bindTexture(this.chartProgram, "uValues", drawLevel.values, 0);
    this.bindTexture(this.chartProgram, "uMetadata", drawLevel.metadata, 1);
    this.bindTexture(this.chartProgram, "uScaleValues", scaleLevel.values, 2);
    this.bindTexture(
      this.chartProgram,
      "uScaleMetadata",
      scaleLevel.metadata,
      3,
    );
    this.uniformInt(this.chartProgram, "uWidth", drawLevel.width);
    this.uniformInt(this.chartProgram, "uCount", drawCount);
    this.uniformInt(
      this.chartProgram,
      "uOldest",
      drawLevelIndex === 0 ? this.oldest() : 0,
    );
    this.uniformInt(this.chartProgram, "uRing", drawLevelIndex === 0 ? 1 : 0);
    this.uniformInt(this.chartProgram, "uScaleWidth", scaleLevel.width);
    this.uniformInt(
      this.chartProgram,
      "uScaleOldest",
      scaleLevelIndex === 0 ? this.oldest() : 0,
    );
    this.uniformInt(
      this.chartProgram,
      "uScaleRing",
      scaleLevelIndex === 0 ? 1 : 0,
    );
    this.uniformFloat(
      this.chartProgram,
      "uPixelRatio",
      this.registration.pixelRatio,
    );
    this.uniformColor(this.registration.palette.accent);
    this.drawMode(0, gl.LINES, drawCount * 2);

    if (drawCount > 1) {
      this.drawMode(1, gl.LINES, (drawCount - 1) * 2);
    }

    if (drawLevelIndex === 0 && drawCount > 1) {
      const segments = Math.max(
        1,
        Math.floor(this.registration.canvas.width / (drawCount - 1)),
      );
      this.uniformInt(this.chartProgram, "uCurveSegments", segments);
      this.drawMode(2, gl.LINE_STRIP, (drawCount - 1) * (segments + 2));
    }

    if (this.registration.rug) {
      this.drawMode(3, gl.POINTS, drawCount);
    }

    this.drawMode(4, gl.LINES, 2);
  }

  private drawMode(mode: number, primitive: number, count: number) {
    this.uniformInt(this.chartProgram, "uMode", mode);
    this.gl.drawArrays(primitive, 0, count);
  }

  private oldest() {
    return (this.head - this.count + this.capacity) & (this.capacity - 1);
  }

  private bindTexture(
    program: WebGLProgram,
    name: string,
    textureValue: WebGLTexture,
    unit: number,
  ) {
    this.gl.activeTexture(this.gl.TEXTURE0 + unit);
    this.gl.bindTexture(this.gl.TEXTURE_2D, textureValue);
    this.gl.uniform1i(this.uniformLocation(program, name), unit);
  }

  private uniformInt(program: WebGLProgram, name: string, value: number) {
    this.gl.uniform1i(this.uniformLocation(program, name), value);
  }

  private uniformFloat(program: WebGLProgram, name: string, value: number) {
    this.gl.uniform1f(this.uniformLocation(program, name), value);
  }

  private uniformColor(value: [number, number, number, number]) {
    this.gl.uniform4fv(
      this.uniformLocation(this.chartProgram, "uColor"),
      value,
    );
  }

  private uniformLocation(program: WebGLProgram, name: string) {
    let locations = this.uniformLocations.get(program);

    if (locations === undefined) {
      locations = new Map();
      this.uniformLocations.set(program, locations);
    }

    const existing = locations.get(name);

    if (existing !== undefined) {
      return existing;
    }

    const location = this.gl.getUniformLocation(program, name);

    if (location === null) {
      throw new Error(`stream renderer uniform is missing: ${name}`);
    }

    locations.set(name, location);
    return location;
  }
}
