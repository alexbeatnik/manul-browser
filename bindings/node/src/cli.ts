#!/usr/bin/env node
/**
 * The `manul` command.
 *
 * A package that carries the engine but leaves it buried in `node_modules` is
 * only half an install: `manul run checkout.hunt` is how most people meet this
 * project. So the entry point is a shim that hands the whole command line to
 * the engine and gets out of the way — every subcommand, every flag, no wrapper
 * parsing anything it does not have to.
 */

import { spawn } from 'node:child_process';
import { constants } from 'node:os';

import { findBinary } from './binary.js';
import { EngineNotFound } from './errors.js';

export function main(argv: string[] = process.argv.slice(2)): void {
  let binary: string;
  try {
    binary = findBinary();
  } catch (err) {
    if (err instanceof EngineNotFound) {
      process.stderr.write(`manul: ${err.message}\n`);
      process.exitCode = 127;
      return;
    }
    throw err;
  }

  const child = spawn(binary, argv, { stdio: 'inherit' });

  // A signal meant for `manul` is meant for the engine. Without passing it on,
  // stopping this shim — a supervisor ending `manul daemon`, a test runner
  // timing out a run — left the engine running as an orphan, still holding its
  // browser and the caller's pipes. A terminal's Ctrl+C reaches both anyway;
  // the engine handles a second one itself.
  const forwarded = ['SIGINT', 'SIGTERM', 'SIGHUP'] as const;
  const forward = (signal: NodeJS.Signals): void => {
    child.kill(signal);
  };
  for (const signal of forwarded) process.on(signal, forward);

  let failedToStart = false;
  child.on('error', (err) => {
    failedToStart = true;
    process.stderr.write(`manul: cannot run ${binary}: ${err.message}\n`);
    process.exitCode = 126;
  });
  child.on('close', (code, signal) => {
    for (const s of forwarded) process.off(s, forward);
    // A spawn that failed still closes, with the negated errno as its code;
    // letting that through turned the 126 above into 243.
    if (failedToStart) return;
    // Mirror the shell's convention, 128 + the signal number, so a killed
    // engine is distinguishable from one that exited cleanly.
    process.exitCode = signal ? 128 + (constants.signals[signal] ?? 0) : (code ?? 0);
  });
}

main();
