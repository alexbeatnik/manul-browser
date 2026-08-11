/**
 * A fake engine that speaks the protocol.
 *
 * The tests drive this instead of the real binary, so they need neither Chrome
 * nor a compiled engine and finish in about a second. It is the Node twin of
 * `bindings/python/tests/fake_engine.py`, and it exists for the same reason:
 * the binding's job is the wire, so the wire is what the tests should exercise.
 *
 * It is a fake, not a mock — it answers the real shapes from `spec/protocol.md`.
 * Behaviour is steered by `MANUL_FAKE_MODE` so one script covers the awkward
 * cases too (a bad protocol version, an engine that dies early).
 */

import { createInterface } from 'node:readline';

type Json = unknown;

const mode = process.env['MANUL_FAKE_MODE'] ?? 'normal';

function send(payload: Record<string, Json>): void {
  process.stdout.write(JSON.stringify(payload) + '\n');
}

if (mode === 'silent') {
  // Exits without a ready event, to prove the binding reports a startup failure
  // rather than hanging on the first request.
  process.exit(3);
}

send({
  event: 'ready',
  protocol: mode === 'future' ? '2.0' : '1.0',
  engine: '0.1.0',
});

/** Variables the fake keeps for `vars` / `set-vars`. */
const vars: Record<string, string> = { greeting: 'hello' };

/** Registrations the client published, so invocations can be aimed correctly. */
let registered: { controls: Array<{ page: string; target: string }>; calls: string[]; hooks: Array<{ kind: string; tag: string }> } = {
  controls: [],
  calls: [],
  hooks: [],
};

let pendingInvoke: ((msg: Record<string, Json>) => void) | null = null;
let invokeSeq = 0;

/**
 * Issue a reverse call and wait for the client's reply.
 *
 * Mirrors the engine: the exchange is nested inside the request the client is
 * already waiting on, and `page.*` is served while it is outstanding.
 */
function invoke(req: Record<string, Json>): Promise<Record<string, Json>> {
  invokeSeq += 1;
  send({ invoke: invokeSeq, ...req });
  return new Promise((resolve) => {
    pendingInvoke = resolve;
  });
}

async function handle(msg: Record<string, Json>): Promise<void> {
  const id = msg['id'];
  const cmd = String(msg['cmd'] ?? '');
  const args = (msg['args'] ?? {}) as Record<string, Json>;

  const ok = (result: Json): void => send({ id, ok: true, result });
  const fail = (code: string, message: string): void =>
    send({ id, ok: false, error: { code, message } });

  switch (cmd) {
    case 'schema':
      return ok({ version: '0.1.0', commands: ['CLICK', 'FILL'] });

    case 'open':
      return ok({ mode: String(args['mode'] ?? 'launch'), cdp: String(args['cdp'] ?? ''), url: 'about:blank' });

    case 'register':
      registered = {
        controls: (args['controls'] ?? []) as Array<{ page: string; target: string }>,
        calls: (args['calls'] ?? []) as string[],
        hooks: (args['hooks'] ?? []) as Array<{ kind: string; tag: string }>,
      };
      return ok({
        controls: registered.controls.length,
        calls: registered.calls.length,
        hooks: registered.hooks.length,
      });

    case 'map':
      return ok({
        url: 'https://example.com/',
        groups: [
          {
            name: 'main',
            truncated: 0,
            elements: [
              { label: 'Sign in', role: 'button', editable: false },
              { label: 'Email', role: 'textbox', editable: true },
            ],
          },
          { name: 'nav', truncated: 2, elements: [{ label: 'Home', role: 'link', editable: false }] },
        ],
      });

    case 'read':
      if (args['selector']) return ok({ text: 'selector text', selector: args['selector'] });
      if (args['label'] === 'Missing') return ok({ value: '', found: false, reason: 'not_found' });
      return ok({ value: 'read value', found: true, reason: '' });

    case 'run-step': {
      const step = String(args['step'] ?? '');
      if (!step) return fail('bad_request', 'run-step needs a step');
      if (step.includes('EXPLODE')) return fail('step_failed', 'deliberate failure');
      // A step naming a registered control triggers the reverse call, which is
      // how the tests reach the handler dispatch path.
      const control = registered.controls.find((c) => step.includes(c.target));
      if (control) {
        const reply = await invoke({
          kind: 'custom_control',
          page: control.page,
          target: control.target,
          action: 'click',
          value: '',
          step,
          vars: { ...vars },
        });
        if (!reply['ok']) {
          const err = (reply['error'] ?? {}) as Record<string, Json>;
          return fail('step_failed', String(err['message'] ?? 'handler failed'));
        }
        return ok({ ok: true, step, action: 'custom_control', url: 'https://example.com/' });
      }
      return ok({
        ok: !step.includes('NOPE'),
        step,
        action: 'click',
        value: '',
        url: 'https://example.com/',
        reason: step.includes('NOPE') ? 'not_found' : '',
        error: '',
        score: 0.98,
        near: [],
      });
    }

    case 'call-host': {
      // Not a real protocol command: a test hook so a CALL handler can be
      // exercised without a .hunt file.
      const reply = await invoke({
        kind: 'call',
        name: String(args['name'] ?? ''),
        args: (args['args'] ?? []) as string[],
        vars: { ...vars },
      });
      if (!reply['ok']) {
        const err = (reply['error'] ?? {}) as Record<string, Json>;
        return fail('step_failed', String(err['message'] ?? 'handler failed'));
      }
      return ok(reply['result']);
    }

    case 'fire-hook': {
      // Likewise: fire one suite hook and report what it published.
      const reply = await invoke({
        kind: 'hook',
        hook: String(args['hook'] ?? 'before_all'),
        tag: String(args['tag'] ?? ''),
        vars: { ...vars },
      });
      if (!reply['ok']) {
        const err = (reply['error'] ?? {}) as Record<string, Json>;
        return fail('step_failed', String(err['message'] ?? 'handler failed'));
      }
      return ok(reply['result']);
    }

    case 'run':
      if (!args['source'] && !args['path']) return fail('bad_request', 'run needs either path or source');
      return ok({ ok: true, url: 'https://example.com/', total_steps: 3, passed: 3, failed: 0 });

    case 'run-suite': {
      const paths = (args['paths'] ?? []) as string[];
      if (paths.length === 0) return fail('bad_request', 'run-suite needs at least one path');
      return ok({
        ok: true,
        total: paths.length,
        passed: paths.length,
        failed: 0,
        skipped: 0,
        hunts: paths.map((p) => ({
          path: p,
          ok: true,
          skipped: false,
          tags: ['smoke'],
          steps: 2,
          passed: 2,
          failed: 0,
          error: '',
        })),
      });
    }

    case 'vars': {
      const set = (args['set'] ?? null) as Record<string, string> | null;
      if (set) Object.assign(vars, set);
      const get = (args['get'] ?? null) as string[] | null;
      if (get) return ok(Object.fromEntries(get.map((k) => [k, vars[k] ?? ''])));
      return ok({ ...vars });
    }

    case 'state':
      return ok({ url: 'https://example.com/', title: 'Example' });

    case 'page.eval':
      return ok(`evaluated:${String(args['js'] ?? '')}`);

    case 'page.url':
      return ok('https://example.com/');

    case 'close':
      ok({});
      process.exit(0);
      return;
  }
  return fail('bad_request', `unknown cmd ${JSON.stringify(cmd)}`);
}

const rl = createInterface({ input: process.stdin, crlfDelay: Infinity });

// Requests are handled one at a time, as the real engine does: a reverse call
// blocks the command that triggered it, and nothing else may run meanwhile.
let chain: Promise<unknown> = Promise.resolve();

rl.on('line', (raw) => {
  const line = raw.trim();
  if (!line) return;
  let msg: Record<string, Json>;
  try {
    msg = JSON.parse(line) as Record<string, Json>;
  } catch {
    return;
  }

  // A reply to an outstanding reverse call is not a request.
  if ('invoke' in msg && pendingInvoke) {
    const resolve = pendingInvoke;
    pendingInvoke = null;
    resolve(msg);
    return;
  }

  // While an invocation is outstanding the engine serves the page primitives
  // inline, off the queue. Queueing them would deadlock: the command holding
  // the queue is precisely the one waiting for this handler to answer.
  if (pendingInvoke && String(msg['cmd'] ?? '').startsWith('page.')) {
    void handle(msg);
    return;
  }

  chain = chain.then(() => handle(msg)).catch(() => undefined);
});

rl.on('close', () => process.exit(0));
