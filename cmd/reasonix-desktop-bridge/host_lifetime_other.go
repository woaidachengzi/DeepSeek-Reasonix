//go:build !darwin

package main

import "os"

func nativeReadinessParent(string) os.FileInfo        { return nil }
func removeNativeReadinessParent(string, os.FileInfo) {}
func nativeOutputPipeGuard(int) func()                { return func() {} }
