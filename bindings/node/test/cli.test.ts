import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';

const CLI = fileURLToPath(new URL('../src/cli.js', import.meta.url));

function alive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

// `manul daemon` runs until it is told to stop. A supervisor that stops the
// `manul` command stops this shim — and the engine it started used to carry on
// as an orphan, holding its browser and the supervisor's pipes open.
describe('manul command', { skip: process.platform === 'win32' && 'POSIX signals only' }, () => {
  it('passes SIGTERM on to the engine', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'manul-cli-'));
    try {
      const pidFile = join(dir, 'pid');
      const engine = join(dir, 'manul');
      writeFileSync(engine, `#!/bin/sh\necho $$ > "${pidFile}"\nexec sleep 30\n`);
      chmodSync(engine, 0o755);

      const shim = spawn(process.execPath, [CLI, 'daemon', 'hunts/'], {
        env: { ...process.env, MANUL_BINARY: engine },
        stdio: 'ignore',
      });
      for (let i = 0; i < 100 && !existsSync(pidFile); i++) await new Promise((r) => setTimeout(r, 20));
      const enginePid = Number(readFileSync(pidFile, 'utf8').trim());
      assert.ok(alive(enginePid), 'precondition: the engine is running');

      const exited = new Promise<number | null>((r) => shim.on('exit', (code) => r(code)));
      shim.kill('SIGTERM');
      const code = await exited;

      for (let i = 0; i < 100 && alive(enginePid); i++) await new Promise((r) => setTimeout(r, 20));
      assert.equal(alive(enginePid), false, 'the engine outlived the manul command');
      assert.equal(code, 143, 'a signalled engine exits 128 + signal number');
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it('passes SIGINT on, and exits 130', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'manul-cli-'));
    try {
      const ready = join(dir, 'ready');
      const engine = join(dir, 'manul');
      writeFileSync(engine, `#!/bin/sh\ntouch "${ready}"\nexec sleep 30\n`);
      chmodSync(engine, 0o755);

      const shim = spawn(process.execPath, [CLI], {
        env: { ...process.env, MANUL_BINARY: engine },
        stdio: 'ignore',
      });
      for (let i = 0; i < 100 && !existsSync(ready); i++) await new Promise((r) => setTimeout(r, 20));
      const exited = new Promise<number | null>((r) => shim.on('exit', (code) => r(code)));
      shim.kill('SIGINT');
      assert.equal(await exited, 130);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it('hands the engine the whole command line, untouched', () => {
    const dir = mkdtempSync(join(tmpdir(), 'manul-cli-'));
    try {
      const out = join(dir, 'argv');
      const engine = join(dir, 'manul');
      writeFileSync(engine, `#!/bin/sh\nfor a in "$@"; do printf '%s\\n' "$a" >> "${out}"; done\n`);
      chmodSync(engine, 0o755);

      const argv = ['run', 'hunts/', '--headless', 'with space', '--tags=a,b'];
      const res = spawnSync(process.execPath, [CLI, ...argv], {
        env: { ...process.env, MANUL_BINARY: engine },
      });
      assert.equal(res.status, 0);
      assert.deepEqual(readFileSync(out, 'utf8').split('\n').slice(0, -1), argv);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("exits with the engine's own status", () => {
    const dir = mkdtempSync(join(tmpdir(), 'manul-cli-'));
    try {
      const engine = join(dir, 'manul');
      writeFileSync(engine, '#!/bin/sh\nexit 7\n');
      chmodSync(engine, 0o755);
      const res = spawnSync(process.execPath, [CLI, 'run', 'x.hunt'], {
        env: { ...process.env, MANUL_BINARY: engine },
      });
      assert.equal(res.status, 7);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it('exits 127 and says where it looked when there is no engine', () => {
    const dir = mkdtempSync(join(tmpdir(), 'manul-cli-'));
    try {
      const missing = join(dir, 'nope', 'manul');
      const res = spawnSync(process.execPath, [CLI, 'run'], {
        env: { ...process.env, MANUL_BINARY: missing, PATH: join(dir, 'empty') },
        encoding: 'utf8',
      });
      assert.equal(res.status, 127);
      assert.match(res.stderr, /^manul: could not find the manul engine/);
      assert.ok(res.stderr.includes(`${missing} (MANUL_BINARY)`));
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it('exits 126 when the engine is there but cannot be run', () => {
    const dir = mkdtempSync(join(tmpdir(), 'manul-cli-'));
    try {
      const engine = join(dir, 'manul');
      writeFileSync(engine, '#!/bin/sh\nexit 0\n');
      chmodSync(engine, 0o644);
      const res = spawnSync(process.execPath, [CLI], {
        env: { ...process.env, MANUL_BINARY: engine },
        encoding: 'utf8',
      });
      assert.equal(res.status, 126);
      assert.ok(res.stderr.includes(`manul: cannot run ${engine}`), res.stderr);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
