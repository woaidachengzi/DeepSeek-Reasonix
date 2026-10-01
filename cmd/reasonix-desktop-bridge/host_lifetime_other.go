//go:build !darwin

package main

import (
	"context"
	"os"
)

func nativeReadinessParent(string) os.FileInfo                                   { return nil }
func removeNativeReadinessParent(string, os.FileInfo)                            {}
func nativeOutputPipeGuard(int) func()                                           { return func() {} }
func nativeLaunchLease(config) *startupLease                                     { return nil }
func nativeStartupBoundary(context.Context, config, *startupLease, string) error { return nil }
func nativeStartupBeforeHostCheck(config, *startupLease) error                   { return nil }
