// Suite hooks — around a whole run, or around every hunt with a given tag.
//
// They fire under session.runSuite([...]) and under
// `manul run hunts/ --hooks manul_hooks.mjs`. A single session.run is one hunt,
// not a suite, so it fires none of them.
//
//   beforeAll    on failure: suite aborts, afterAll still runs
//   beforeGroup  on failure: that hunt is skipped, the rest runs
//   after*       on failure: reported, changes no result

import { afterAll, afterGroup, beforeAll, beforeGroup } from 'manul-browser';

export const EVENTS = [];

function log(msg) {
  EVENTS.push(msg);
  // stderr: under --hooks stdout is the protocol (serveHooks redirects
  // console.log anyway; being explicit costs nothing).
  console.error(`[hooks] ${msg}`);
}

// Global variables: visible as {placeholders} in every hunt of the suite.
beforeAll((ctx) => {
  ctx.set('base_url', process.env.DEMO_BASE_URL ?? 'http://localhost:8766');
  ctx.set('env_name', process.env.DEMO_ENV ?? 'local');
  ctx.set('signer', 'Manul QA');
  ctx.metadata.started = Date.now(); // scratch space, never reaches a hunt
  log('beforeAll: globals published');
});

// A second beforeAll in the same slot sees what the first one set.
beforeAll((ctx) => {
  if (!ctx.variables.base_url) throw new Error('the globals hook must run first');
  ctx.set('api_token', `tok-${ctx.variables.env_name}-42`);
  log('beforeAll: token issued');
});

afterAll((ctx) => log(`afterAll: suite done, token was ${ctx.variables.api_token}`));

beforeGroup('auth', (ctx) => {
  ctx.set('login_user', 'ada');
  ctx.set('login_password', 'lovelace');
  log('beforeGroup(auth)');
});

afterGroup('auth', () => log('afterGroup(auth)'));

beforeGroup('smoke', (ctx) => {
  ctx.set('run_kind', 'smoke');
  log('beforeGroup(smoke)');
});

// Refuse hunts tagged `quarantine` — they are counted as skipped.
beforeGroup('quarantine', () => {
  log('beforeGroup(quarantine): refusing');
  throw new Error('quarantined: known-flaky, skipped on purpose');
});
