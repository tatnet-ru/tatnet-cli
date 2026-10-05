package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

func (e *Env) filterApps(ctx context.Context, apps []any, repo, branch, name, domain string) ([]any, error) {
	out := []any{}
	for _, a := range apps {
		if (repo != "" && output.Value(a, "repo_full_name") != repo) || (branch != "" && output.Value(a, "branch") != branch) || (name != "" && output.Value(a, "name") != name) {
			continue
		}
		if domain != "" {
			c, err := e.Client()
			if err != nil {
				return nil, err
			}
			ds, err := paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
				return call(c.AppsListDomainsByIdWithResponse(ctx, output.Value(a, "id"), &tatnet.AppsListDomainsByIdParams{Limit: &size, Offset: &offset}))
			}, 0, 0)
			if err != nil {
				return nil, err
			}
			found := false
			for _, d := range ds {
				if strings.EqualFold(strings.TrimSuffix(output.Value(d, "domain"), "."), strings.TrimSuffix(domain, ".")) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func appWaitCommand(e *Env) *cobra.Command {
	var commit, build string
	var deadline time.Duration
	var logs bool
	cmd := &cobra.Command{Use: "wait <приложение>", Short: "Дождаться live конкретной сборки или коммита", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if (commit == "") == (build == "") {
			return fmt.Errorf("укажите ровно один из --commit или --build")
		}
		if deadline <= 0 {
			return fmt.Errorf("--deadline должен быть > 0")
		}
		c, project, id, err := e.appTarget(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		b, err := e.waitLive(cmd, c, project, id, build, commit, deadline, logs)
		if err != nil {
			return err
		}
		return e.Printer.Object(b, []output.Column{output.Col("сборка", "id"), output.Col("коммит", "commit_sha"), output.Col("статус", "status"), output.Col("деплой", "deploy_state")})
	}}
	cmd.Flags().StringVar(&commit, "commit", "", "полный SHA коммита (точное совпадение)")
	cmd.Flags().StringVar(&build, "build", "", "id сборки")
	cmd.Flags().DurationVar(&deadline, "deadline", 30*time.Minute, "предельное время ожидания")
	cmd.Flags().BoolVar(&logs, "logs", false, "печатать лог выбранной сборки")
	return cmd
}

// Once selected, a build stays pinned: a newer build must never satisfy this wait.
func (e *Env) waitLive(cmd *cobra.Command, c *tatnet.ClientWithResponses, project, appID, buildID, commit string, timeout time.Duration, logs bool) (any, error) {
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	observed := &cobra.Command{}
	observed.SetContext(ctx)
	observed.SetOut(cmd.OutOrStdout())
	observed.SetErr(cmd.ErrOrStderr())
	shown := 0
	started := false
	for {
		selected, err := findObservedBuild(ctx, c, appID, buildID, commit)
		if err != nil {
			return nil, err
		}
		if selected != nil {
			buildID = output.Value(selected, "id")
		}

		if selected != nil {
			status, state := output.Value(selected, "status"), output.Value(selected, "deploy_state")
			failed := status == "error" || status == "failed" || status == "cancelled" || status == "canceled" || state == "failing" || state == "never_booted" || state == "build_failed" || state == "stale_serving" || state == "booted"
			if failed {
				e.printMissedLog(observed, c, project, appID, buildID, true, shown)
				return nil, fmt.Errorf("выкат сборки %s не удался (status=%s, deploy_state=%s): %s; %s", buildID, status, state, output.Value(selected, "error"), output.Value(selected, "boot_error"))
			}
			if logs && !started {
				started = true
				shown, err = e.streamBuildLog(observed, c, project, appID, buildID, 0)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "предупреждение: лог прервался: %v\n", err)
				}
				continue
			}
			if status == "success" && state == "live" {
				e.printMissedLog(observed, c, project, appID, buildID, logs, shown)
				return selected, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("live не подтверждён для сборки %s (коммит %s): %w", buildID, commit, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func findObservedBuild(ctx context.Context, c *tatnet.ClientWithResponses, appID, buildID, commit string) (any, error) {
	for offset := 0; ; offset += 100 {
		size := 100
		v, err := call(c.AppsListBuildsByIdWithResponse(ctx, appID, &tatnet.AppsListBuildsByIdParams{Limit: &size, Offset: &offset}))
		if err != nil {
			return nil, err
		}
		page := items(v)
		for _, b := range page {
			if (buildID != "" && output.Value(b, "id") == buildID) || (buildID == "" && output.Value(b, "commit_sha") == commit) {
				return b, nil
			}
		}
		if len(page) < size {
			return nil, nil
		}
	}
}
