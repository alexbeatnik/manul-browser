// The exceptions, and when each one is thrown.

import { join } from 'node:path';
import * as manul from 'manul-browser';
import { ROOT, banner, check, withAppAndSession } from './_common.mjs';

banner('EngineNotFound — the binary could not be located');
// findBinary tries: binary › $MANUL_BINARY › the platform package › PATH. A
// missing explicit path falls through to the next candidate, so this only
// throws where nothing else is installed (a source checkout).
const saved = { bin: process.env.MANUL_BINARY, path: process.env.PATH };
delete process.env.MANUL_BINARY;
process.env.PATH = '';
try {
  const found = manul.findBinary('/definitely/not/manul');
  console.log(`  fell through to ${found} (installed package) — EngineNotFound not reachable here`);
} catch (e) {
  check(e instanceof manul.EngineNotFound, `EngineNotFound: ${e.message.split('\n')[0]}`);
} finally {
  process.env.PATH = saved.path;
  if (saved.bin) process.env.MANUL_BINARY = saved.bin;
}

banner('ProtocolError — something that is not the engine');
try {
  await manul.Session.launch({ binary: [process.execPath, '-e', "console.log('hello, I am not manul')"] });
  check(false, 'expected ProtocolError');
} catch (e) {
  check(e instanceof manul.ProtocolError, `ProtocolError: ${e.message.slice(0, 70)}…`);
}

banner('EngineError — the engine rejected a request; the session survives');
let session;
await withAppAndSession({}, async (app, s) => {
  session = s;
  try {
    await s.readText('div[[[');
  } catch (e) {
    check(e instanceof manul.EngineError && e.code === 'bad_request', `bad selector → ${e.code}`);
  }
  try {
    await s.run({ path: join(ROOT, 'hunts', 'missing.hunt') });
  } catch (e) {
    check(e.code === 'bad_request', `missing hunt → ${e.code}`);
  }
  try {
    await s.transport.call('no-such-command');
  } catch (e) {
    check(e.code === 'bad_request', `unknown command → ${e.code}`);
  }
  check((await s.step(`NAVIGATE to ${app.baseUrl}/index.html`)).ok, 'session still usable');
});

banner('SessionClosed — after close()');
await session.close(); // closing twice is safe
try {
  await session.state();
  check(false, 'expected SessionClosed');
} catch (e) {
  check(e instanceof manul.SessionClosed && session.closed, 'SessionClosed thrown, session.closed is true');
}

banner('await using — the session closes itself');
{
  await using s = await manul.Session.launch({ cwd: ROOT, stderr: 'ignore' });
  check(s.opened, 'opened inside the block');
  session = s;
}
check(session.closed, 'closed when the block ended');
check(new manul.EngineError('x', 'y') instanceof manul.ManulError, 'every error derives from ManulError');
