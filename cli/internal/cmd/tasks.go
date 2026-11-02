package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simiooo/pinelink/cli/internal/client"
)

func newTasksCommand(opts *options) *cobra.Command {
	root := &cobra.Command{Use: "tasks", Short: "inspect and control transfer tasks"}
	root.AddCommand(newTaskListCommand(opts), newTaskProjectionCommand(opts))
	for _, action := range []string{"get", "pause", "resume", "cancel", "delete", "fail"} {
		root.AddCommand(newTaskActionCommand(opts, action))
	}
	return root
}

func newTaskListCommand(opts *options) *cobra.Command {
	var kind, state string
	var includeDeleted bool
	var limit int
	command := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			list, err := api.ListTasks(cmd.Context(), opts.owner, opts.device, kind, state, includeDeleted, limit)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(list)
			}
			for _, task := range list.Tasks {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s %d/%d v%d\n", task.ID, task.Kind, task.State, task.ProgressBytes, task.TotalBytes, task.Version)
			}
			return nil
		},
	}
	command.Flags().StringVar(&kind, "kind", "", "UPLOAD or DOWNLOAD")
	command.Flags().StringVar(&state, "state", "", "filter state")
	command.Flags().BoolVar(&includeDeleted, "include-deleted", false, "include deleted tasks")
	command.Flags().IntVar(&limit, "limit", 50, "maximum entries")
	return command
}

func newTaskActionCommand(opts *options, action string) *cobra.Command {
	return &cobra.Command{
		Use:  action + " TASK_ID",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			task, err := api.GetTask(cmd.Context(), args[0], opts.owner, opts.device)
			if err != nil {
				return err
			}
			if action != "get" {
				task, err = api.TaskAction(cmd.Context(), args[0], action, opts.owner, opts.device, task.Version)
				if err != nil {
					return err
				}
			}
			if opts.jsonOutput {
				return opts.output(task)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "task_id=%s\nreference_id=%s\nstate=%s\nversion=%d\n", task.ID, task.ReferenceID, task.State, task.Version)
			return nil
		},
	}
}

func newTaskProjectionCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:  "projection TASK_ID",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			projection, err := api.TaskProjection(cmd.Context(), args[0], opts.owner, opts.device)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(projection)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "state=%s\nprogress=%d/%d\nversion=%d\n", projection.State, projection.ProgressBytes, projection.TotalBytes, projection.Version)
			return nil
		},
	}
}

func markTaskFailed(cmd *cobra.Command, opts *options, api *client.Client, taskID, reason string) {
	if taskID == "" {
		return
	}
	task, err := api.GetTask(cmd.Context(), taskID, opts.owner, opts.device)
	if err != nil || (task.State != "RUNNING" && task.State != "QUEUED") {
		return
	}
	_, _ = api.TaskAction(cmd.Context(), task.ID, "fail", opts.owner, opts.device, task.Version, reason)
}
