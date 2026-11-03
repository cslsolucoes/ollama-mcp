//go:build race

package treino

// raceEnabled: the race detector slows the code 20 to 30 times, so timing targets do not apply.
const raceEnabled = true
