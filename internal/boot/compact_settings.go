package boot

import (
	"reasonix/internal/agent"
	"reasonix/internal/config"
)

func withCompactionOptions(opts agent.Options, cfg config.AgentConfig, source func() float64) agent.Options {
	opts.SoftCompactRatio = cfg.SoftCompactRatio
	opts.ToolResultSnipRatio = cfg.ToolResultSnipRatio
	opts.CompactRatio = cfg.CompactRatio
	opts.CompactRatioSource = source
	opts.CompactForceRatio = cfg.CompactForceRatio
	opts.ContextEditing = cfg.ContextEditing
	opts.RecentKeep = cfg.RecentKeep
	opts.ArchiveDir = config.ArchiveDir()
	return opts
}
