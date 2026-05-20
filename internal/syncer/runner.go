package syncer

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Summary is the outcome of a sync run.
type Summary struct {
	Copied int
	Failed int
}

// Runner executes sync jobs by invoking rsync.
type Runner struct{}

// NewRunner returns a Runner.
func NewRunner() *Runner { return &Runner{} }

// rsyncArgs builds the rsync argument list for a job. The "/./" pivot makes
// rsync recreate the job's relative path (parents included) under the dest.
func rsyncArgs(j Job) []string {
	return []string{
		"-rtR", "--modify-window=1",
		"--no-perms", "--no-owner", "--no-group",
		"--partial-dir=.rsync-partial", "--info=progress2",
		"--exclude=.DS_Store", "--exclude=._*", "--exclude=@eaDir",
		j.Source + "/./" + j.Path,
		j.Dest + "/",
	}
}

// Run copies each job in order, emitting events through emit. A failed job is
// recorded and the run continues.
func (r *Runner) Run(ctx context.Context, jobs []Job, emit func(Event)) Summary {
	emit(Event{Type: EvRunStart, TotalEntries: len(jobs)})
	var s Summary
	for i, j := range jobs {
		if ctx.Err() != nil {
			break // context cancelled — remaining jobs left unattempted
		}
		emit(Event{Type: EvEntryStart, Collection: j.Collection, Path: j.Path,
			DoneEntries: i, TotalEntries: len(jobs)})
		err := r.runOne(ctx, j, emit)
		done := Event{Type: EvEntryDone, Collection: j.Collection, Path: j.Path,
			DoneEntries: i + 1, TotalEntries: len(jobs)}
		if err != nil {
			s.Failed++
			done.Err = err.Error()
		} else {
			s.Copied++
			done.Percent = 100
		}
		emit(done)
	}
	emit(Event{Type: EvRunDone, Copied: s.Copied, Failed: s.Failed,
		TotalEntries: len(jobs), DoneEntries: len(jobs)})
	return s
}

// runOne runs rsync for a single job, streaming progress events.
func (r *Runner) runOne(ctx context.Context, j Job, emit func(Event)) error {
	cmd := exec.CommandContext(ctx, "rsync", rsyncArgs(j)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	// rsync writes progress lines separated by carriage returns.
	sc := bufio.NewScanner(stdout)
	sc.Split(scanCROrLF)
	for sc.Scan() {
		if p, ok := ParseProgress(sc.Text()); ok {
			emit(Event{Type: EvEntryProgress, Collection: j.Collection, Path: j.Path,
				Percent: p.Percent, Rate: p.Rate, ETA: p.ETA})
		}
	}
	if scanErr := sc.Err(); scanErr != nil {
		_ = cmd.Wait() // reap the process before returning
		return fmt.Errorf("reading rsync output: %w", scanErr)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("rsync: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// scanCROrLF is a bufio.SplitFunc that splits on either \r or \n, so that
// rsync's carriage-return-updated progress lines are read individually.
func scanCROrLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
