package sshtest_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/provider"
	"reasonix/internal/remote/bootstrap"
	"reasonix/internal/remote/sshtest"
	"reasonix/internal/store"
)

// Opt-in package integration: no bridge/controller factory or HTTP image body
// mock. Production bootstrap commands execute on a private SSH shell and launch
// the current CLI's actual Serve. The image arrives through packaged WKWebView.
func TestNativePackageRemoteImageActualSSHServe(t *testing.T) {
	app, cli := os.Getenv("REASONIX_NATIVE_REMOTE_IMAGE_APP"), os.Getenv("REASONIX_NATIVE_REMOTE_IMAGE_CLI")
	if app == "" {
		t.Skip("requires explicit owned macOS package acceptance")
	}
	if runtime.GOOS != "darwin" || !filepath.IsAbs(app) || !filepath.IsAbs(cli) {
		t.Fatal("invalid native fixture paths/platform")
	}
	info, err := os.Stat(cli)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		t.Fatal("owned CLI missing")
	}
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("/private/tmp", "reasonix-native-ssh-")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Private SSH/Serve evidence:", root)
	var receipt map[string]any
	var serveStopped bool
	t.Cleanup(func() {
		if receipt == nil || t.Failed() {
			return
		}
		if !serveStopped {
			t.Error("owned Serve shutdown not confirmed")
			return
		}
		receipt["ownedServeStopped"] = true
		data, err := json.Marshal(receipt)
		if err == nil {
			err = os.WriteFile(filepath.Join(root, "integration-receipt.json"), data, 0600)
		}
		if err != nil {
			t.Error("integration receipt unavailable")
		}
	})
	home, core, workspace, bin := filepath.Join(root, "remote-home"), filepath.Join(root, "remote-core"), filepath.Join(root, "workspace"), filepath.Join(root, "bin")
	for _, dir := range []string{home, core, workspace, bin} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("REASONIX_HOME", core)
	t.Setenv("REASONIX_STATE_HOME", core)
	if err := os.Symlink(cli, filepath.Join(bin, "reasonix")); err != nil {
		t.Fatal(err)
	}
	var modelRequests atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelRequests.Add(1)
		http.Error(w, "owned readonly fixture: model calls refused", 503)
	}))
	t.Cleanup(model.Close)
	remoteConfig := "default_model=\"local/alpha\"\n[[providers]]\nname=\"local\"\nkind=\"openai\"\nbase_url=" + strconv.Quote(model.URL+"/v1") + "\napi_key_env=\"REASONIX_OWNED_MODEL_KEY\"\nmodels=[\"alpha\"]\ndefault=\"alpha\"\n"
	if err := os.WriteFile(config.UserConfigPath(), []byte(remoteConfig), 0600); err != nil {
		t.Fatal(err)
	}
	sessions := config.ProjectSessionDir(workspace)
	if err := os.MkdirAll(sessions, 0700); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(sessions, "owned-image.jsonl")
	session := agent.NewSession("owned fixture")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "owned screenshot"})
	if err := session.Save(sessionPath); err != nil {
		t.Fatal(err)
	}
	if err := agent.UpdateBranchMeta(sessionPath, false, func(meta *agent.BranchMeta) error {
		meta.WorkspaceRoot = workspace
		meta.CustomTitle = "owned screenshot"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	metaBefore, err := os.ReadFile(agent.BranchMetaPath(sessionPath))
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 16, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 16; x++ {
			pixels.SetNRGBA(x, y, color.NRGBA{R: uint8(40 + x), G: uint8(80 + y), B: 180, A: 255})
		}
	}
	var raster bytes.Buffer
	if err := png.Encode(&raster, pixels); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "owned.png"), raster.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "owned native image fixture")
	if err != nil {
		t.Fatal(err)
	}
	identity := filepath.Join(root, "owned-identity.pem")
	if err := os.WriteFile(identity, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	shellEnv := []string{"HOME=" + home, "REASONIX_HOME=" + core, "REASONIX_STATE_HOME=" + core, "PATH=" + bin + ":/usr/bin:/bin:/usr/sbin:/sbin", "ENV=/dev/null", "BASH_ENV=/dev/null", "REASONIX_OWNED_MODEL_KEY=owned-fixture-not-real"}
	runShell := func(ctx context.Context, command string) (string, string, int) {
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
		cmd.Env = shellEnv
		cmd.Dir = workspace
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			code = 1
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			}
		}
		return stdout.String(), stderr.String(), code
	}
	var execs atomic.Int32
	server := sshtest.Start(t, sshtest.Options{AuthorizedKey: signer.PublicKey(), SFTPRoot: home, Exec: func(command string) (string, string, int) {
		execs.Add(1)
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		return runShell(ctx, command)
	}})
	_, portString, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	remoteDir := filepath.Join(home, ".reasonix", store.RemoteDirName)
	slug := store.RemoteWorkspaceSlug(workspace)
	paths := bootstrap.StatePaths{Dir: remoteDir, StateJSON: filepath.Join(remoteDir, store.RemoteServeStateName(slug)), TokenFile: filepath.Join(remoteDir, store.RemoteServeTokenName(slug)), PortFile: filepath.Join(remoteDir, store.RemoteServePortName(slug)), PidFile: filepath.Join(remoteDir, store.RemoteServePidName(slug))}
	// Stop only a confirmed fixture Serve. Production StopCommand fences PID reuse
	// using its own unique token/port filenames; never signal a process by name.
	t.Cleanup(func() {
		data, _ := os.ReadFile(paths.PidFile)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid <= 1 {
			if data, err := os.ReadFile(paths.StateJSON); err == nil {
				var state bootstrap.ServeState
				if json.Unmarshal(data, &state) == nil {
					pid = state.PID
				}
			}
		}
		if pid > 1 {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _, code := runShell(ctx, bootstrap.StopCommand(pid, paths))
			if code != 0 {
				t.Error("owned Serve cleanup failed")
				return
			}
			for attempt := 0; attempt < 20; attempt++ {
				alive, _, code := runShell(ctx, bootstrap.ServeAliveCommand(pid, paths))
				if code == 0 && strings.TrimSpace(alive) == "0" {
					serveStopped = true
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			t.Error("owned Serve remains alive after cleanup")
		}
	})
	data, err := json.Marshal(map[string]any{"port": port, "fingerprint": ssh.FingerprintSHA256(server.HostKey.PublicKey()), "identityFile": identity, "workspace": workspace, "sessionPath": sessionPath, "source": "owned.png", "width": 16, "height": 10})
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "owned-remote-manifest.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	python := os.Getenv("REASONIX_NATIVE_REMOTE_IMAGE_PYTHON")
	if python == "" {
		python = "/opt/homebrew/bin/python3"
	}
	template := os.Getenv("REASONIX_NATIVE_REMOTE_IMAGE_WINDOW_TEMPLATE")
	if !filepath.IsAbs(template) {
		t.Fatal("owned geometry template required")
	}
	cmd := exec.CommandContext(ctx, python, filepath.Join(repo, "tools/tauri/smoke-native-remote-image-positive.py"), app, "--fixture-manifest", manifest, "--window-state-template", template, "--profile", "both")
	cmd.Env = []string{"PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin", "LANG=en_US.UTF-8"}
	cmd.Dir = repo
	output, runErr := cmd.CombinedOutput()
	if err := os.WriteFile(filepath.Join(root, "native.log"), output, 0600); err != nil {
		t.Fatal(err)
	}
	if runErr != nil {
		t.Fatal("native positive probe failed; fixed-status log retained in private evidence")
	}
	stateData, err := os.ReadFile(paths.StateJSON)
	if err != nil {
		t.Fatal("production bootstrap state missing")
	}
	var state bootstrap.ServeState
	if json.Unmarshal(stateData, &state) != nil || state.Workspace != workspace || state.PID <= 1 || !strings.HasPrefix(state.Addr, "127.0.0.1:") {
		t.Fatal("production Serve state invalid")
	}
	if execs.Load() < 4 {
		t.Fatal("production SSH bootstrap was not exercised")
	}
	alive, _, code := runShell(ctx, bootstrap.ServeAliveCommand(state.PID, paths))
	if code != 0 || strings.TrimSpace(alive) != "1" {
		t.Fatal("production Serve process identity not confirmed")
	}
	after, err := os.ReadFile(sessionPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("readonly probe changed transcript")
	}
	metaAfter, err := os.ReadFile(agent.BranchMetaPath(sessionPath))
	if err != nil || !bytes.Equal(metaBefore, metaAfter) {
		t.Fatal("readonly probe changed saved workspace metadata")
	}
	if modelRequests.Load() != 0 {
		t.Fatal("readonly image flow invoked model")
	}
	receipt = map[string]any{"ok": true, "realSSH": true, "privateKeyAuth": true, "actualShellBootstrap": true, "productionServe": true, "realPackagedBridge": true, "realWKWebViewPixels": true, "managedAndExplicit": true, "savedSessionUnchanged": true, "zeroModelRequests": true, "clipboardTouched": false, "sharedTranscriptUI": false}
}
