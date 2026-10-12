// Command reasonix-profile-takeover validates exports against a verified backup
// and prepares either a private stage or the explicitly selected original root.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"reasonix/internal/sessionidentity"
	"reasonix/internal/takeover"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	snapshot := flag.String("snapshot", "", "verified offline snapshot")
	exports := flag.String("exports", "", "read-only Wails codec exports")
	format := flag.String("format", "classic", "current Wails storage: classic or v5")
	profile := flag.String("profile", "", "original or staged destination profile")
	apply := flag.Bool("apply", false, "explicitly write new Tauri state; otherwise review only")
	flag.Parse()
	ctx := context.Background()
	manifest, err := sessionidentity.VerifyOfflineSnapshot(ctx, *snapshot)
	if err != nil {
		return err
	}
	var rows []takeover.Row
	if *format == "classic" {
		var supplementary []takeover.Row
		if *exports != "" {
			supplementary, err = takeover.Plan(filepath.Join(*snapshot, "profile"), manifest.SourceProfile, *exports)
			if err != nil {
				return err
			}
		}
		rows, err = takeover.ClassicPlan(filepath.Join(*snapshot, "profile"), manifest.SourceProfile, supplementary)
	} else if *format == "v5" {
		rows, err = takeover.Plan(filepath.Join(*snapshot, "profile"), manifest.SourceProfile, *exports)
	} else {
		return fmt.Errorf("format must be classic or v5")
	}
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.OriginalRelativePath != "" {
			continue
		}
		path := filepath.Join(*snapshot, "profile", "desktop-sessions-v5", "by-id", row.SourceID, "events.frames")
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != row.Export.SourceSHA256 {
			return fmt.Errorf("export no longer matches verified source log: %s", row.SourceID)
		}
	}
	archived := 0
	originalPaths := 0
	for _, row := range rows {
		if row.OriginalRelativePath != "" {
			originalPaths++
		}
		if row.Archived {
			archived++
		}
	}
	if !*apply {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"reviewed": len(rows), "originalPaths": originalPaths, "v5Fallback": len(rows) - originalPaths, "archived": archived, "sourceProfile": manifest.SourceProfile})
	}
	if !filepath.IsAbs(*profile) {
		return fmt.Errorf("apply requires an absolute --profile")
	}
	receipt, err := takeover.WriteVerified(ctx, *profile, *snapshot, rows)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"completed": len(receipt.Rows), "archived": archived, "profile": receipt.ProfileRoot, "receipt": filepath.Join(receipt.ProfileRoot, "tauri-takeover-v1.json")})
}
