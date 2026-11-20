//go:build race

package engine

// raceSlowdown widens timing assertions: the race detector slows I/O-heavy tests several times.
const raceSlowdown = 4
