package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/config"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/model"
	workload "github.com/Hitesh-s0lanki/system-arch/load-tester/k6"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Runner talks to the local Engine through the Docker CLI, without a shell.
type Runner struct{ Config config.Config }

func Name(id string) string { return "load-test-" + id }
func command(ctx context.Context, args ...string) (string, error) {
	var b cappedBuffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = &b
	cmd.Stderr = &b
	e := cmd.Run()
	if e != nil {
		return b.String(), fmt.Errorf("docker %s: %w: %s", args[0], e, b.String())
	}
	return strings.TrimSpace(b.String()), nil
}

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 1024*1024 {
		keep := min(n, 1024*1024-b.Len())
		_, _ = b.Buffer.Write(p[:keep])
	}
	return n, nil
}
func (r *Runner) Ready(ctx context.Context) error {
	_, e := command(ctx, "info", "--format", "{{.ServerVersion}}")
	return e
}
func (r *Runner) Cleanup(ctx context.Context, id string) error {
	// Only remove a container carrying the expected ownership label.
	out, e := command(ctx, "inspect", "--format", `{{index .Config.Labels "local.load-tester.job"}}`, Name(id))
	if e != nil {
		if strings.Contains(out, "No such object") || strings.Contains(out, "No such container") {
			return nil
		}
		return e
	}
	if out != id {
		return fmt.Errorf("refusing to remove container with mismatched ownership")
	}
	_, e = command(ctx, "rm", "-f", Name(id))
	return e
}
func (r *Runner) Run(ctx context.Context, j model.Job) (code int, err error) {
	dir := filepath.Join(r.Config.DataDir, j.ID)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return -1, err
	}
	b, err := json.Marshal(j.Config)
	if err != nil {
		return -1, err
	}
	if err = os.WriteFile(filepath.Join(dir, "config.json"), b, 0600); err != nil {
		return -1, err
	}
	defer os.Remove(filepath.Join(dir, "config.json"))
	if err = os.WriteFile(filepath.Join(dir, "script.js"), workload.Script, 0644); err != nil {
		return -1, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		logs, _ := command(cleanupCtx, "logs", Name(j.ID))
		logs = strings.ReplaceAll(logs, j.Config.TargetURL, "[target]")
		for _, secret := range j.Config.Headers {
			if secret != "" {
				logs = strings.ReplaceAll(logs, secret, "[redacted]")
			}
		}
		_ = os.WriteFile(filepath.Join(dir, "logs.txt"), []byte(logs), 0600)
		if e := r.Cleanup(cleanupCtx, j.ID); e != nil && err == nil {
			err = fmt.Errorf("container cleanup failed: %w", e)
		}
	}()
	// Image pulling is explicit, so an absent image works on the first request too.
	if _, e := command(ctx, "image", "inspect", r.Config.Image); e != nil {
		if _, e = command(ctx, "pull", r.Config.Image); e != nil {
			return -1, e
		}
	}
	args := []string{"create", "--name", Name(j.ID), "--label", "local.load-tester.job=" + j.ID,
		"--network", r.Config.Network, "--add-host", "host.docker.internal:host-gateway",
		"--mount", "type=volume,src=" + r.Config.Volume + ",dst=/data",
		"--workdir", "/data/" + j.ID, "--user", "0:0", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--memory", "512m", "--cpus", "1", "--pids-limit", "128", "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		r.Config.Image, "run", "--out", "json=metrics.json", "script.js"}
	if _, err = command(ctx, args...); err != nil {
		return -1, err
	}
	if _, err = command(ctx, "start", Name(j.ID)); err != nil {
		return -1, err
	}
	out, err := command(ctx, "wait", Name(j.ID))
	if err != nil {
		return -1, err
	}
	code, err = strconv.Atoi(out)
	if err != nil {
		return -1, fmt.Errorf("invalid container exit code: %q", out)
	}
	return code, nil
}
