// Dependency acceptance probe, not a SYMM correctness or migration test.
// Exercises the installed library over a real TCP socket without changing it.
import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
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

// Add a valid unreferenced segment; no schema-level payload is rewritten.
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
  const transport = await EzRpcTransport.connect('127.0.0.1', server.address().port);
  transport.onError = (error) => errors.push(error.message);
  const received = [];
  let failure = null;
  try {
    for (const frame of frames) {
      // This is only the transport control. Cross-implementation assertions
      // below and in check-capnp-frame.go use independent expected values.
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
await probe('valid two-segment frame', [multi]);
await probe('two-segment frame followed by single-segment frame', [multi, single]);
const cross = { name: 'native Go bootstrap decoded by TypeScript', expected: 41, actual: null, passed: false, failure: null };
try {
  const bytes = await readFile(path.join(output, 'go-bootstrap.capnp'));
  const decoded = deserializeRpcMessage(new Uint8Array(bytes));
  assert.equal(decoded.type, 'bootstrap');
  cross.actual = decoded.bootstrap.questionId;
  assert.equal(cross.actual, cross.expected, 'question ID must survive crossing implementations');
  cross.passed = true;
} catch (error) {
  cross.failure = error.stack ?? String(error);
}
observations.push(cross);
console.log(JSON.stringify(cross));
await writeFile(path.join(output, 'typescript-results.json'), JSON.stringify({
  dependency: '@naeemo/capnp@0.9.3',
  node: process.version,
  observations,
}, null, 2));
process.exitCode = observations.every((result) => result.passed) ? 0 : 1;
