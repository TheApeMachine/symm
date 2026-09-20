// Dependency acceptance probe, not a SYMM correctness or migration test.
// Exercises the installed library over a real TCP socket without changing it.
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import net from 'node:net';
import path from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { EzRpcTransport, serializeRpcMessage, deserializeRpcMessage } from '@naeemo/capnp';

const output = process.argv[2];
if (!output) throw new Error('usage: node probe.mjs OUTPUT_DIRECTORY');
await mkdir(output, { recursive: true });
const observations = [];

function bootstrap(questionId) {
  return Buffer.from(serializeRpcMessage({ type: 'bootstrap', bootstrap: { questionId } }));
}

// A valid Capnp stream frame with an extra unreferenced segment. Existing
// segment-relative pointers retain their offsets. The segment table is padded
// to an eight-byte boundary as required by the standard framing.
function twoSegments(single) {
  assert.equal(single.readUInt32LE(0), 0, 'fixture must start as a single segment');
  const frame = Buffer.alloc(single.length + 16);
  frame.writeUInt32LE(1, 0);
  frame.writeUInt32LE(single.readUInt32LE(4), 4);
  frame.writeUInt32LE(1, 8);
  frame.writeUInt32LE(0, 12);
  single.copy(frame, 16, 8);
  return frame;
}

async function probe(name, frames, fragmented = false) {
  const errors = [];
  const sockets = new Set();
  const server = net.createServer((socket) => {
    sockets.add(socket);
    socket.on('error', (error) => errors.push(error.message));
    socket.on('close', () => sockets.delete(socket));
    void (async () => {
      const bytes = Buffer.concat(frames);
      if (!fragmented) {
        socket.write(bytes);
        return;
      }
      for (let offset = 0; offset < bytes.length; offset += 3) {
        if (socket.destroyed) return;
        socket.write(bytes.subarray(offset, offset + 3));
        await sleep(1);
      }
    })().catch((error) => errors.push(error.message));
  });
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  const port = server.address().port;
  const transport = await EzRpcTransport.connect('127.0.0.1', port);
  transport.onError = (error) => errors.push(error.message);
  const received = [];
  let failure = null;
  try {
    for (const frame of frames) {
      const expected = deserializeRpcMessage(new Uint8Array(frame));
      const deadline = new AbortController();
      let actual;
      try {
        actual = await Promise.race([
          transport.receive(),
          sleep(2000, undefined, { signal: deadline.signal }).then(() => {
            throw new Error('No message delivered within 2 seconds');
          }),
        ]);
      } finally {
        deadline.abort();
      }
      assert.equal(actual?.type, 'bootstrap');
      assert.equal(actual.bootstrap.questionId, expected.bootstrap.questionId);
      received.push(actual.bootstrap.questionId);
    }
  } catch (error) {
    failure = error.stack ?? String(error);
  } finally {
    transport.close();
    for (const socket of sockets) socket.destroy();
    await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  }
  const result = { name, passed: failure === null, received, errors, failure };
  observations.push(result);
  console.log(JSON.stringify(result));
}

const single = bootstrap(41);
const fragmented = bootstrap(42);
const multi = twoSegments(bootstrap(43));
await writeFile(path.join(output, 'single.capnp'), single);
await writeFile(path.join(output, 'multi.capnp'), multi);
await probe('single-segment control', [single]);
await probe('fragmented single-segment control', [fragmented], true);
await probe('two complete messages in one socket write', [single, fragmented]);
await probe('valid two-segment message', [multi]);
await probe('two-segment message followed by single-segment message', [multi, single]);
await writeFile(path.join(output, 'typescript-results.json'), JSON.stringify({
  dependency: '@naeemo/capnp@0.9.3',
  node: process.version,
  observations,
}, null, 2));
process.exitCode = observations.every((result) => result.passed) ? 0 : 1;
