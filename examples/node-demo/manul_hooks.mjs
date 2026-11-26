// Hook script for the CLI: `manul run hunts/ --hooks manul_hooks.mjs`.
//
// The engine spawns this file (with $MANUL_NODE, else `node`) and calls back
// into it. It reuses the very same handlers the scripts use — registering them
// is just importing them.

import { serveHooks } from 'manul-browser';
import './src/handlers.mjs';

await serveHooks(); // blocks until the engine closes stdin; must be last
