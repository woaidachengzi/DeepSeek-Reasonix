package main

import (
	"context"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/netclient"
)

func loadPreviewRemoteImage(ctx context.Context, source string) desktopbridge.WorkspaceImageView {
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return desktopbridge.WorkspaceImageView{ErrorCode: "proxy-config"}
	}
	client, err := netclient.NewPublicImageClient(cfg.NetworkProxySpec(), nil)
	if err != nil {
		return desktopbridge.WorkspaceImageView{ErrorCode: "proxy-config"}
	}
	return desktopbridge.FetchRemoteWorkspaceImage(ctx, source, client)
}
