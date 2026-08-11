package runtime

// Vars returns a flattened snapshot of every variable visible to the next step,
// with the same precedence the DSL itself sees. The map is a copy — mutating it
// does not touch runtime state.
func (rt *Runtime) Vars() map[string]string {
	return rt.vars.Flatten()
}

// SetVar seeds a variable at mission level, the scope a whole hunt run shares.
// Row- and step-scoped values still shadow it, which is what a caller seeding a
// value before a run wants: a per-row value from a data file should win.
func (rt *Runtime) SetVar(name, value string) {
	rt.vars.Set(name, value, LevelMission)
}

// SetGlobalVar seeds a variable at global scope — the level suite-level hooks
// publish into. Everything a hunt sets for itself, and every per-row value from
// a data file, still shadows it, so a global default never overrides the
// specific case it was meant to back up.
func (rt *Runtime) SetGlobalVar(name, value string) {
	rt.vars.Set(name, value, LevelGlobal)
}

// SetGlobalVars seeds a whole map at global scope.
func (rt *Runtime) SetGlobalVars(kv map[string]string) {
	for k, v := range kv {
		rt.SetGlobalVar(k, v)
	}
}
