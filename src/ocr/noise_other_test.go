//go:build !windows

package ocr

func emitNoise() {}

func commitMemory(uintptr) error { return nil }
