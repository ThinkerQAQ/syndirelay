import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { needsBridgeRestart } from '../bridge-version.js';

test('upgrade only a stale Bridge when the on-disk native host is already current', () => {
  assert.equal(needsBridgeRestart('0.1.195', '0.1.195', '0.1.194'), true);
  assert.equal(needsBridgeRestart('0.1.195', '0.1.195', '0.1.195'), false);
  assert.equal(needsBridgeRestart('0.1.195', '0.1.194', '0.1.194'), false);
  assert.equal(needsBridgeRestart('0.1.195', '', '0.1.194'), false);
  assert.equal(needsBridgeRestart('0.1.195', '0.1.195', ''), false);
});

test('bridge reconciliation is single-flight and delegates to the authenticated restart route', () => {
  const file=path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../background.js');
  const source=fs.readFileSync(file, 'utf8');
  assert.match(source, /needsBridgeRestart\(/);
  assert.match(source, /bridgeVersionRecoveryKey/);
  assert.match(source, /bridgeVersionRecoveryPromise/);
  assert.match(source, /await restartBridge\(\)/);
  assert.match(source, /\/v1\/restart/);
});
