package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/nalanj/agent-queue/client"
	"github.com/spf13/cobra"
)

var (
	runInterval = 30 * time.Second // How often to extend while running
)

var runCmd = &cobra.Command{
	Use:   "run -- <command> [args...]",
	Short: "Dequeue a job and run a command with the body as stdin",
	Long: `Dequeues the next pending job and runs a command with the job body
piped to stdin. The job claim is extended periodically while the command
runs to prevent timeout.

On success, the job is deleted.
On failure, the job is left in processing state (will timeout and retry).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("command required")
		}

		c := client.New()

		// Dequeue a job
		job, err := c.Dequeue()
		if err != nil {
			return fmt.Errorf("dequeue failed: %w", err)
		}
		if job == nil {
			fmt.Println("queue empty")
			return nil
		}

		fmt.Printf("Running job %d...\n", job.ID)

		// Start extending in background
		stopCh := make(chan struct{})
		stoppedCh := make(chan struct{})
		go func() {
			defer close(stoppedCh)
			ticker := time.NewTicker(runInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_, err := c.Extend(job.ID)
					if err != nil {
						// Job was deleted - stop extending silently
						close(stopCh)
						return
					}
				case <-stopCh:
					return
				}
			}
		}()

		// Run the command with job body as stdin
		err = runCommand(job.Body, args)

		// Stop the extend goroutine
		close(stopCh)
		<-stoppedCh

		if err != nil {
			fmt.Fprintf(os.Stderr, "Command failed: %v\n", err)
			// Leave job in processing state - it will timeout and retry
			return nil
		}

		// Success - try to delete the job (may already be deleted)
		if err := c.Delete(job.ID); err != nil && !errors.Is(err, client.ErrJobNotFound) {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete job: %v\n", err)
		}
		fmt.Printf("Job %d completed.\n", job.ID)

		return nil
	},
}

func init() {
	RootCmd.AddCommand(runCmd)
}

// runCommand executes a command with body piped to stdin
func runCommand(body string, args []string) error {
	cmd := exec.Command(args[0], args[1:]...)

	// Create a pipe for stdin
	r, w := io.Pipe()
	cmd.Stdin = r
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Write body in goroutine and close when done
	go func() {
		defer w.Close()
		if body != "" {
			w.Write([]byte(body))
		}
	}()

	return cmd.Run()
}
