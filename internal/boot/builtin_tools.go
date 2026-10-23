package boot

import (
	"fmt"
	"io"
	"time"

	"reasonix/internal/netclient"
	"reasonix/internal/sandbox"
	"reasonix/internal/sessiontemp"
	"reasonix/internal/tool"
	"reasonix/internal/tool/builtin"
)

// addBuiltins adds enabled built-in tools to reg. An empty list means all of
// them. writeRoots confines the file-writing built-ins to the workspace: after
// the (unconfined) defaults are added, each enabled writer is replaced by an
// instance bound to writeRoots (preserving registry order).
// forbidReadRoots confines the read/list/search built-ins so they cannot peek at
// the listed directories.
// When workDir is non-empty, tools resolve relative paths against it instead of
// the process cwd, enabling concurrent multi-project sessions.
// sessionGuard blocks writer-tool targets inside Reasonix's own session stores
// and makes bash warn when a command references them. managedConfig names the
// Reasonix-owned config files writable outside writeRoots after a fresh
// per-write human approval.
func addBuiltins(reg *tool.Registry, enabled, writeRoots []string, writeRootSet *sandbox.WritableRootSet, bashSpec sandbox.Spec, bashTimeout time.Duration, searchSpec builtin.SearchSpec, stderr io.Writer, workDir string, proxySpec netclient.ProxySpec, forbidReadRoots []string, readPathResolver *builtin.PathResolver, sessionGuard builtin.SessionDataGuard, managedConfig builtin.ManagedConfigPaths, overlay builtin.FileOverlay, terminal builtin.TerminalRunner, sessionTemp *sessiontemp.Manager, fileWriteReceipt func(path string, hadPrior bool, prior []byte)) {
	// If a workspace directory is set, use workspace-bound tools that resolve
	// paths relative to that directory. Otherwise fall back to the process-cwd
	// compile-time builtins.
	if workDir != "" {
		ws := builtin.Workspace{Dir: workDir, WriteRoots: writeRoots, WriteRootSet: writeRootSet, ForbidReadRoots: forbidReadRoots, Bash: bashSpec, BashTimeout: bashTimeout, Search: searchSpec, ProxySpec: proxySpec, ReadPaths: readPathResolver, SessionGuard: sessionGuard, ManagedConfig: managedConfig, FileOverlay: overlay, Terminal: terminal, SessionTemp: sessionTemp, FileWriteReceipt: fileWriteReceipt}
		for _, t := range ws.Tools(enabled...) {
			reg.Add(t)
		}
		return
	}

	if len(enabled) == 0 {
		for _, t := range tool.Builtins() {
			reg.Add(t)
		}
	} else {
		for _, name := range enabled {
			if t, ok := tool.LookupBuiltin(name); ok {
				reg.Add(t)
			} else {
				fmt.Fprintf(stderr, "warning: unknown built-in tool %q\n", name)
			}
		}
	}
	// Rebind enabled tools to the same sandbox, root, image, and desktop
	// capability policies used by Workspace.Tools. Registry order is preserved.
	bashTool := builtin.ConfineBash(bashSpec, sessionGuard, bashTimeout)
	if rebound, ok := builtin.BindSessionTemp(bashTool, sessionTemp); ok {
		bashTool = rebound
	}
	searchTool := builtin.ConfineSearch(searchSpec, bashSpec, forbidReadRoots)
	if rebound, ok := builtin.BindSessionTemp(searchTool, sessionTemp); ok {
		searchTool = rebound
	}
	writers := builtin.ConfineWriters(writeRoots, sessionGuard, managedConfig)
	for i, writer := range writers {
		writers[i] = builtin.BindFileWriteReceipt(writer, fileWriteReceipt)
	}
	confined := append(writers,
		builtin.ConfineComputerUse(bashSpec),
		bashTool,
		searchTool,
		builtin.ConfineWebFetch(proxySpec))
	confined = append(confined, builtin.ConfineReaders(forbidReadRoots)...)
	for i, tl := range confined {
		confined[i] = builtin.BindWriteRootSet(tl, writeRootSet)
	}
	for _, t := range confined {
		if _, ok := reg.Get(t.Name()); ok {
			reg.Add(t)
		}
	}
}
