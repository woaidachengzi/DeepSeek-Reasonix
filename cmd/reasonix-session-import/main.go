// Command reasonix-session-import reviews scan-only transcripts in an offline
// snapshot. It never imports into the source profile or a running Tauri host.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"reasonix/internal/sessionidentity"
)

const stageMarkerName = ".reasonix-session-import-stage.json"
const maxReviewFile = 8 << 20

type stageMarker struct {
	Version       int    `json:"version"`
	Stage         string `json:"stage"`
	Snapshot      string `json:"snapshot"`
	SourceProfile string `json:"sourceProfile"`
}

type candidateChoice struct {
	ID               string  `json:"id"`
	File             string  `json:"file"`
	TranscriptSHA256 string  `json:"transcriptSha256"`
	Title            *string `json:"title"`
	WorkspaceRoot    *string `json:"workspaceRoot"`
	Selected         bool    `json:"selected"`
}

type selectionDocument struct {
	Candidates []candidateChoice `json:"candidates"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: reasonix-session-import snapshot|stage|list|review|apply [flags]")
	}
	switch args[0] {
	case "snapshot":
		flags := flag.NewFlagSet("snapshot", flag.ContinueOnError)
		profile := flags.String("profile", "", "stopped source profile")
		catalog := flags.String("catalog", "", "host workbench-sessions.json")
		outParent := flags.String("out-parent", "", "existing destination directory outside source")
		stopped := flags.Bool("writers-stopped", false, "confirm all old and new profile writers are stopped")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		if !*stopped {
			return errors.New("snapshot requires --writers-stopped; old Wails writers do not honor the profile gate")
		}
		snapshot, err := sessionidentity.CreateOfflineSnapshot(ctx, *profile, *catalog, *outParent)
		if err != nil {
			return err
		}
		_, err = sessionidentity.VerifyOfflineSnapshot(ctx, snapshot)
		if err != nil {
			return err
		}
		return writeJSON(output, map[string]string{"snapshot": snapshot})
	case "stage":
		flags := flag.NewFlagSet("stage", flag.ContinueOnError)
		snapshot := flags.String("snapshot", "", "verified snapshot directory")
		outParent := flags.String("out-parent", "", "existing destination directory")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		manifest, err := sessionidentity.VerifyOfflineSnapshot(ctx, *snapshot)
		if err != nil {
			return err
		}
		stage, err := sessionidentity.StageOfflineSnapshot(ctx, *snapshot, *outParent)
		if err != nil {
			return err
		}
		snapshotPath, err := filepath.Abs(*snapshot)
		if err != nil {
			return err
		}
		snapshotPath, err = filepath.EvalSymlinks(snapshotPath)
		if err != nil {
			return err
		}
		marker := stageMarker{Version: 1, Stage: stage, Snapshot: snapshotPath, SourceProfile: manifest.SourceProfile}
		if err := writePrivateNew(filepath.Join(stage, stageMarkerName), marker); err != nil {
			return err
		}
		return writeJSON(output, map[string]string{"stage": stage})
	case "list":
		flags := flag.NewFlagSet("list", flag.ContinueOnError)
		stage := flags.String("stage", "", "staged recovery directory")
		selectionOut := flags.String("selection-out", "", "new selection template JSON file")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		root, marker, err := verifyStage(ctx, *stage)
		if err != nil {
			return err
		}
		document, issues, err := listCandidates(ctx, root)
		if err != nil {
			return err
		}
		if *selectionOut != "" {
			if err := safeReviewOutput(*selectionOut, root, marker.SourceProfile, marker.Snapshot); err != nil {
				return err
			}
			if err := writePrivateNew(*selectionOut, document); err != nil {
				return err
			}
		}
		return writeJSON(output, struct {
			Candidates []candidateChoice `json:"candidates"`
			Issues     []string          `json:"issues"`
		}{document.Candidates, issues})
	case "review":
		flags := flag.NewFlagSet("review", flag.ContinueOnError)
		stage := flags.String("stage", "", "staged recovery directory")
		selection := flags.String("selection", "", "edited selection template")
		planOut := flags.String("plan-out", "", "new review plan JSON file")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		root, marker, err := verifyStage(ctx, *stage)
		if err != nil {
			return err
		}
		if *selection == "" || *planOut == "" {
			return errors.New("review requires --selection and --plan-out")
		}
		if err := safeReviewOutput(*planOut, root, marker.SourceProfile, marker.Snapshot); err != nil {
			return err
		}
		var document selectionDocument
		if err := readStrictJSON(*selection, &document); err != nil {
			return err
		}
		selected := make([]sessionidentity.ScanImportSelection, 0)
		for _, item := range document.Candidates {
			if !item.Selected {
				continue
			}
			selected = append(selected, sessionidentity.ScanImportSelection{
				ID: item.ID, Title: item.Title, WorkspaceRoot: item.WorkspaceRoot, TranscriptSHA256: item.TranscriptSHA256,
			})
		}
		identities, _, err := openReadOnlyIdentity(ctx, root)
		if err != nil {
			return err
		}
		if identities != nil {
			defer identities.Close()
		}
		plan, err := sessionidentity.PrepareScanImportReview(ctx, identities, sessionDir(root), catalogPath(root), selected)
		if err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return err
		}
		encoded = append(encoded, '\n')
		if err := writePrivateBytesNew(*planOut, encoded); err != nil {
			return err
		}
		digest := sha256.Sum256(encoded)
		return writeJSON(output, map[string]any{"plan": *planOut, "approve": hex.EncodeToString(digest[:]), "count": len(plan.Rows)})
	case "apply":
		flags := flag.NewFlagSet("apply", flag.ContinueOnError)
		stage := flags.String("stage", "", "staged recovery directory")
		planPath := flags.String("plan", "", "approved review plan")
		approve := flags.String("approve", "", "SHA-256 printed by review")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		root, _, err := verifyStage(ctx, *stage)
		if err != nil {
			return err
		}
		if *planPath == "" || *approve == "" {
			return errors.New("apply requires --plan and --approve")
		}
		encoded, err := readPrivateBytes(*planPath)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(encoded)
		if hex.EncodeToString(digest[:]) != *approve {
			return errors.New("review plan approval SHA-256 does not match")
		}
		var plan sessionidentity.ScanImportReview
		if err := decodeStrict(encoded, &plan); err != nil {
			return err
		}
		if plan.SessionDir != sessionDir(root) || plan.CatalogPath != catalogPath(root) {
			return errors.New("review plan does not belong to this staged profile")
		}
		readOnly, _, err := openReadOnlyIdentity(ctx, root)
		if err != nil {
			return err
		}
		fresh, err := sessionidentity.PrepareScanImportReview(ctx, readOnly, plan.SessionDir, plan.CatalogPath, plan.Selected)
		if readOnly != nil {
			_ = readOnly.Close()
		}
		if err != nil || !reflect.DeepEqual(fresh, plan) {
			return fmt.Errorf("%w: staged data changed after review", sessionidentity.ErrImportReviewChanged)
		}
		identities, err := sessionidentity.Open(ctx, identityPath(root), root)
		if err != nil {
			return err
		}
		defer identities.Close()
		result, err := identities.ApplyScanImportReview(ctx, plan)
		if err != nil {
			return err
		}
		return writeJSON(output, map[string]any{"applied": result.Applied, "stage": *stage})
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	return nil
}

func verifyStage(ctx context.Context, stage string) (string, stageMarker, error) {
	if strings.TrimSpace(stage) == "" {
		return "", stageMarker{}, errors.New("staged recovery directory is required")
	}
	root, err := filepath.Abs(stage)
	if err != nil {
		return "", stageMarker{}, err
	}
	if !strings.HasPrefix(filepath.Base(root), "reasonix-session-recovery-") {
		return "", stageMarker{}, errors.New("directory was not created by offline staging")
	}
	var marker stageMarker
	if err := readStrictJSON(filepath.Join(root, stageMarkerName), &marker); err != nil {
		return "", stageMarker{}, err
	}
	if marker.Version != 1 || marker.Stage != root || marker.SourceProfile == "" || marker.Snapshot == "" {
		return "", stageMarker{}, errors.New("staged profile marker is invalid")
	}
	manifest, err := sessionidentity.VerifyOfflineSnapshot(ctx, marker.Snapshot)
	if err != nil || manifest.SourceProfile != marker.SourceProfile {
		return "", stageMarker{}, errors.New("source snapshot cannot be verified")
	}
	resolvedStage, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", stageMarker{}, err
	}
	// The verified snapshot is self-contained; its original profile may have
	// been moved or removed by the time a recovery copy is reviewed.
	resolvedSource := marker.SourceProfile
	if existingSource, err := filepath.EvalSymlinks(marker.SourceProfile); err == nil {
		resolvedSource = existingSource
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", stageMarker{}, err
	}
	if overlaps(resolvedStage, marker.SourceProfile) || overlaps(resolvedStage, resolvedSource) {
		return "", stageMarker{}, errors.New("staged copy overlaps the source profile")
	}
	profile := filepath.Join(root, "profile")
	for _, path := range []string{root, profile, sessionDir(profile), filepath.Join(root, "catalog")} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", stageMarker{}, fmt.Errorf("staged directory %s is missing or not a plain directory: %v", path, err)
		}
	}
	info, err := os.Lstat(catalogPath(profile))
	if err != nil || !info.Mode().IsRegular() {
		return "", stageMarker{}, errors.New("staged catalog is missing or not a regular file")
	}
	return profile, marker, nil
}

func overlaps(a, b string) bool { return within(a, b) || within(b, a) }

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func safeReviewOutput(path, stagedProfile, sourceProfile, snapshot string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("review output path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return err
	}
	resolvedSource := sourceProfile
	if currentSource, err := filepath.EvalSymlinks(sourceProfile); err == nil {
		resolvedSource = currentSource
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if within(sourceProfile, parent) || within(resolvedSource, parent) || within(stagedProfile, parent) || within(snapshot, parent) {
		return errors.New("review output must be outside source profile, staged profile, and snapshot")
	}
	return nil
}

func sessionDir(profile string) string { return filepath.Join(profile, "sessions") }
func catalogPath(profile string) string {
	return filepath.Join(filepath.Dir(profile), "catalog", "workbench-sessions.json")
}
func identityPath(profile string) string {
	return filepath.Join(profile, "desktop", "session-state-v1.sqlite")
}

func openReadOnlyIdentity(ctx context.Context, profile string) (*sessionidentity.Store, bool, error) {
	return sessionidentity.OpenReadOnlyIfExists(ctx, identityPath(profile), profile)
}

func listCandidates(ctx context.Context, profile string) (selectionDocument, []string, error) {
	identities, _, err := openReadOnlyIdentity(ctx, profile)
	if err != nil {
		return selectionDocument{}, nil, err
	}
	if identities != nil {
		defer identities.Close()
	}
	report, err := sessionidentity.Inventory(ctx, identities, sessionDir(profile), catalogPath(profile))
	if err != nil {
		return selectionDocument{}, nil, err
	}
	document := selectionDocument{Candidates: make([]candidateChoice, 0)}
	for _, row := range report.Entries {
		if row.Source != sessionidentity.InventoryFromScan || row.Claim != sessionidentity.ClaimUnclaimed || !row.Exists {
			continue
		}
		digest, err := digestFile(ctx, row.Path)
		if err != nil {
			return selectionDocument{}, nil, err
		}
		document.Candidates = append(document.Candidates, candidateChoice{
			ID: row.ID, File: filepath.Base(row.Path), TranscriptSHA256: digest,
		})
	}
	return document, report.Errors, nil
}

func digestFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("scan candidate is no longer a regular file")
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readPrivateBytes(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, errors.New("review file is missing or not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("review file cannot be opened")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
		return nil, errors.New("review file changed while opening")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("review file has non-private permissions")
	}
	if info.Size() > maxReviewFile {
		return nil, errors.New("review file is too large")
	}
	encoded, err := io.ReadAll(io.LimitReader(file, maxReviewFile+1))
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxReviewFile {
		return nil, errors.New("review file is too large")
	}
	return encoded, nil
}

func readStrictJSON(path string, value any) error {
	encoded, err := readPrivateBytes(path)
	if err != nil {
		return err
	}
	return decodeStrict(encoded, value)
}

func decodeStrict(encoded []byte, value any) error {
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("review file contains trailing data")
	}
	return nil
}

func writePrivateNew(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateBytesNew(path, append(encoded, '\n'))
}

func writePrivateBytesNew(path string, encoded []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeJSON(out io.Writer, value any) error { return json.NewEncoder(out).Encode(value) }
