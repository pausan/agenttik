//go:build !linux && !windows && !darwin

package screen

// openAudio is nil where this machine's sound cannot be recorded yet.
var openAudio func() (Audio, error)
