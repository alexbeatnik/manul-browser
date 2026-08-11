/**
 * NDJSON-over-stdio transport for `manul serve`.
 *
 * This module owns the child process and the wire format, and nothing else. It
 * does not know what a page or a step is — that lives in `session.ts`. Keeping
 * the split means the protocol can gain commands without this file changing.
 *
 * The shape differs from the Python transport in one way that matters. Python
 * blocks on a read and holds a lock; Node cannot block, so replies are matched
 * by id against a pending map. That makes nested calls fall out for free — a
 * `page.eval` issued from inside a handler is just another pending id — but it
 * also means two overlapping top-level commands could each trigger a reverse
 * call, and the protocol's reverse calls are strictly nested. So top-level
 * commands are serialised through a queue, which is what Python's lock does.
 */

import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';

import { findBinary } from './binary.js';
import { EngineError, ProtocolError, SessionClosed } from './errors.js';

/**
 * This binding is written against protocol 1.x. Minor bumps only add things, so
 * they are safe; a major bump changes existing shapes and must be refused.
 */
export const SUPPORTED_PROTOCOL_MAJOR = 1;

export type Json = unknown;

/** An engine-to-client callback: a custom control, CALL handler or suite hook. */
export interface InvokeMessage {
  invoke: number;
  kind: string;
  [key: string]: Json;
}

export type InvokeHandler = (msg: InvokeMessage) => Json | Promise<Json>;

export interface TransportOptions {
  /**
   * The engine to run. A string is a path; an array is taken verbatim as the
   * command prefix, so the engine can be reached through a wrapper — `wsl`,
   * `docker exec`, a shim script — without this package knowing about any of
   * them. Omitted means "find it".
   */
  binary?: string | readonly string[];
  /** Extra arguments appended after `serve --stdio`. */
  args?: readonly string[];
  cwd?: string;
  env?: NodeJS.ProcessEnv;
  /**
   * Where engine logs go. Engine logs are on stderr and never parsed; the
   * default inherits this process's stderr so warnings stay visible.
   */
  stderr?: 'inherit' | 'ignore' | 'pipe';
}

interface Pending {
  resolve: (value: Json) => void;
  reject: (reason: Error) => void;
}

/** A live `manul serve --stdio` process. */
export class Transport {
  /** Protocol version the engine reported in its ready event. */
  protocol = '';
  /** Engine version the engine reported in its ready event. */
  engineVersion = '';

  /**
   * Handles reverse calls — the engine asking this process to run a custom
   * control, a CALL handler or a hook. Set by `Session`; without it an invoke
   * is answered with an error rather than left to hang.
   */
  onInvoke: InvokeHandler | null = null;

  readonly #proc: ChildProcessWithoutNullStreams;
  readonly #path: string;
  readonly #pending = new Map<number, Pending>();
  #buffer = '';
  #nextId = 0;
  #closed = false;
  #exited: Promise<number | null>;
  /** Tail of the serialisation queue: every top-level call links onto it. */
  #queue: Promise<unknown> = Promise.resolve();
  #ready: Promise<void>;

  constructor(options: TransportOptions = {}) {
    const prefix = Array.isArray(options.binary)
      ? [...(options.binary as readonly string[])]
      : [findBinary(options.binary as string | undefined)];
    if (prefix.length === 0) throw new TypeError('binary array is empty');
    this.#path = prefix[0]!;

    const argv = [...prefix.slice(1), 'serve', '--stdio', ...(options.args ?? [])];
    this.#proc = spawn(this.#path, argv, {
      cwd: options.cwd,
      env: options.env,
      stdio: ['pipe', 'pipe', options.stderr ?? 'inherit'],
    }) as ChildProcessWithoutNullStreams;

    this.#proc.stdout.setEncoding('utf8');
    this.#proc.stdout.on('data', (chunk: string) => this.#onData(chunk));

    this.#exited = new Promise((resolve) => {
      this.#proc.once('close', (code) => {
        // Anything still waiting will never be answered. Failing them beats a
        // caller hanging on a promise nothing can settle.
        this.#failAll(
          new ProtocolError(
            `engine at ${this.#path} exited (code ${code}) while a request was pending`,
          ),
        );
        resolve(code);
      });
    });
    this.#proc.once('error', (err) => this.#failAll(new ProtocolError(`engine failed to start: ${err.message}`)));

    this.#ready = this.#awaitReady();
  }

  /** Resolves once the engine's ready event has been read and accepted. */
  ready(): Promise<void> {
    return this.#ready;
  }

  get closed(): boolean {
    return this.#closed;
  }

  // ── lifecycle ─────────────────────────────────────────────────────────────

  /**
   * Consume the ready event the engine always writes first.
   *
   * A process that dies before emitting it is reported as a startup failure
   * rather than left to hang on the first real request.
   */
  async #awaitReady(): Promise<void> {
    const msg = await new Promise<Record<string, Json>>((resolve, reject) => {
      this.#pending.set(-1, {
        resolve: (v) => resolve(v as Record<string, Json>),
        reject,
      });
    });

    if (msg['event'] !== 'ready') {
      throw new ProtocolError(`expected a ready event, got ${JSON.stringify(msg)}`);
    }
    this.protocol = String(msg['protocol'] ?? '');
    this.engineVersion = String(msg['engine'] ?? '');

    const major = this.protocol.split('.', 1)[0];
    if (major !== String(SUPPORTED_PROTOCOL_MAJOR)) {
      throw new ProtocolError(
        `engine speaks protocol ${this.protocol}, this binding supports ` +
          `${SUPPORTED_PROTOCOL_MAJOR}.x — upgrade the manul-browser package`,
      );
    }
  }

  /**
   * Shut the engine down, politely first.
   *
   * `close` gives the engine a chance to release its browser; only a process
   * that ignores that is killed. Safe to call twice.
   */
  async close(timeoutMs = 5000): Promise<void> {
    if (this.#closed) return;
    this.#closed = true;

    try {
      await this.#dispatch('close');
    } catch {
      // The engine may already be gone; that is the outcome we wanted anyway.
    }
    try {
      this.#proc.stdin.end();
    } catch {
      /* already closed */
    }

    const timer = new Promise<'timeout'>((resolve) => {
      const t = setTimeout(() => resolve('timeout'), timeoutMs);
      // Do not hold the event loop open just to wait for a kill deadline.
      if (typeof t.unref === 'function') t.unref();
    });
    if ((await Promise.race([this.#exited, timer])) === 'timeout') {
      this.#proc.kill();
      await this.#exited;
    }
  }

  // ── wire ──────────────────────────────────────────────────────────────────

  /**
   * Send one command and return its result.
   *
   * Rejects with `EngineError` when the engine answers `ok: false` — the
   * session stays usable, so callers may catch it and carry on.
   *
   * Calls are serialised: a second `call` waits for the first to settle. That
   * keeps reverse calls strictly nested, which the protocol requires.
   */
  call(cmd: string, args?: Record<string, Json>): Promise<Json> {
    if (this.#closed) return Promise.reject(new SessionClosed('session is closed'));
    const run = this.#queue.then(
      () => this.#dispatch(cmd, args),
      () => this.#dispatch(cmd, args),
    );
    // The queue tracks completion, not success: one failed command must not
    // poison every command after it.
    this.#queue = run.catch(() => undefined);
    return run;
  }

  /**
   * Issue a request from inside a reverse call.
   *
   * Only the engine's `page.*` primitives are available here — everything else
   * would re-enter the step that is currently running. It bypasses the queue
   * deliberately: the caller is already inside a `call` that holds it, and
   * queueing would deadlock.
   */
  nestedCall(cmd: string, args?: Record<string, Json>): Promise<Json> {
    return this.#dispatch(cmd, args);
  }

  #dispatch(cmd: string, args?: Record<string, Json>): Promise<Json> {
    const id = ++this.#nextId;
    const req: Record<string, Json> = { id, cmd };
    if (args) {
      // Drop unset optionals so the engine sees its own defaults rather than a
      // wall of nulls.
      const filtered = Object.fromEntries(
        Object.entries(args).filter(([, v]) => v !== undefined && v !== null),
      );
      if (Object.keys(filtered).length > 0) req['args'] = filtered;
    }

    return new Promise<Json>((resolve, reject) => {
      this.#pending.set(id, { resolve, reject });
      try {
        this.#write(req);
      } catch (err) {
        this.#pending.delete(id);
        reject(err as Error);
      }
    });
  }

  #write(payload: Record<string, Json>): void {
    if (!this.#proc.stdin.writable) throw new ProtocolError('engine stdin is not available');
    this.#proc.stdin.write(JSON.stringify(payload) + '\n');
  }

  #onData(chunk: string): void {
    this.#buffer += chunk;
    let nl: number;
    while ((nl = this.#buffer.indexOf('\n')) >= 0) {
      const line = this.#buffer.slice(0, nl).trim();
      this.#buffer = this.#buffer.slice(nl + 1);
      if (line) this.#onLine(line);
    }
  }

  #onLine(line: string): void {
    let msg: Record<string, Json>;
    try {
      // A UTF-8 BOM ahead of the first line is common enough from Windows
      // pipelines that rejecting it would only ever look like a bug here.
      msg = JSON.parse(line.replace(/^﻿/, '')) as Record<string, Json>;
    } catch {
      this.#failAll(new ProtocolError(`unreadable line from the engine: ${line}`));
      return;
    }

    // A reverse call: run the handler and answer it. The engine is paused
    // inside our request until we do.
    if ('invoke' in msg) {
      void this.#serveInvoke(msg as unknown as InvokeMessage);
      return;
    }

    // The ready event is the one message with no id; it is claimed by the
    // sentinel the constructor parked.
    if (!('id' in msg) || msg['id'] === null || msg['id'] === undefined) {
      const readyWaiter = this.#pending.get(-1);
      if (readyWaiter) {
        this.#pending.delete(-1);
        readyWaiter.resolve(msg);
      }
      // Any other id-less line is an event this binding does not care about.
      return;
    }

    const id = Number(msg['id']);
    const waiter = this.#pending.get(id);
    if (!waiter) return; // a reply to something nobody is waiting for
    this.#pending.delete(id);

    if (msg['ok']) {
      waiter.resolve(msg['result']);
      return;
    }
    const err = (msg['error'] ?? {}) as Record<string, Json>;
    waiter.reject(
      new EngineError(
        String(err['code'] ?? 'unknown'),
        String(err['message'] ?? 'engine reported a failure'),
      ),
    );
  }

  /**
   * Run one reverse call and write its result back.
   *
   * A handler that throws is reported to the engine, which fails the step — the
   * session itself stays healthy, because a broken handler is a bug in one
   * step, not a reason to tear down the browser.
   */
  async #serveInvoke(msg: InvokeMessage): Promise<void> {
    try {
      if (this.onInvoke === null) {
        throw new Error(`engine asked for ${JSON.stringify(msg.kind)} but no handler is registered`);
      }
      const result = await this.onInvoke(msg);
      this.#write({ invoke: msg.invoke, ok: true, result: result ?? null });
    } catch (exc) {
      const e = exc as Error;
      this.#write({
        invoke: msg.invoke,
        ok: false,
        error: { code: 'handler_failed', message: `${e.name}: ${e.message}` },
      });
    }
  }

  #failAll(err: Error): void {
    for (const [, waiter] of this.#pending) waiter.reject(err);
    this.#pending.clear();
  }
}
