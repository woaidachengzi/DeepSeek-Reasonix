package main

import "reasonix/internal/terminalprocess"

func terminalPlatformAvailable() (bool, string) { return terminalprocess.Available() }

func startTerminalProcess(spec terminalStartSpec) (terminalProcess, error) {
	return terminalprocess.Start(terminalprocess.Spec{
		Path: spec.command.path, Args: spec.command.args, Dir: spec.dir,
		Env: spec.env, Columns: spec.cols, Rows: spec.rows,
	})
}
