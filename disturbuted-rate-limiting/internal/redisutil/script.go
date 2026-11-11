// Package redisutil executes quota mutations without ambiguous retries.
package redisutil

import (
	"context"

	redisclient "github.com/redis/go-redis/v9"
)

// nonRetryCmd also disables Cluster's retry loop on ambiguous I/O failures.
// MOVED/ASK and NOSCRIPT responses are safe: the script did not execute there.
type nonRetryCmd struct{ *redisclient.Cmd }

func (*nonRetryCmd) NoRetry() bool { return true }

// RunOnce only falls back to EVAL when NOSCRIPT guarantees no prior execution.
func RunOnce(ctx context.Context, client redisclient.UniversalClient, script *redisclient.Script, source string, keys []string, args ...any) *redisclient.Cmd {
	invoke := func(command, code string) *redisclient.Cmd {
		argv := []any{command, code, len(keys)}
		for _, key := range keys {
			argv = append(argv, key)
		}
		argv = append(argv, args...)
		cmd := &nonRetryCmd{redisclient.NewCmd(ctx, argv...)}
		_ = client.Process(ctx, cmd)
		return cmd.Cmd
	}
	cmd := invoke("evalsha", script.Hash())
	if redisclient.HasErrorPrefix(cmd.Err(), "NOSCRIPT") {
		return invoke("eval", source)
	}
	return cmd
}
