package remove

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/spf13/cobra"
	"github.com/ublue-os/bext/internal"
	"github.com/ublue-os/bext/pkg/fileio"
	"github.com/ublue-os/bext/pkg/logging"
	percent "github.com/ublue-os/bext/pkg/percentmanager"
)

var RemoveCmd = &cobra.Command{
	Use:   "remove [TARGET...]",
	Short: "Remove a layer from your managed layers",
	Long:  `Remove either an entire layer or a specific hash in cache for that layer`,
	RunE:  removeCmd,
	Args:  cobra.MinimumNArgs(1),
}

var (
	fHash   []string
	fDryRun bool
)

func init() {
	RemoveCmd.Flags().StringSliceVarP(&fHash, "hash", "h", []string{}, "Remove specific hash from storage")
	RemoveCmd.Flags().BoolVar(&fDryRun, "dry-run", false, "Do not remove anything")
}

func removeCmd(cmd *cobra.Command, args []string) error {
	// todo dryrun flag
	slog.Info("Ignoring dryrun flag", "dryrun", fDryRun)

	pw := percent.NewProgressWriter()
	if !*internal.Config.NoProgress {
		go pw.Render()
		slog.SetDefault(logging.NewMuteLogger())
	}
	if len(fHash) > 1 {
		pw.SetNumTrackersExpected(len(fHash))
	} else {
		pw.SetNumTrackersExpected(len(args))
	}
	cache_dir, err := filepath.Abs(path.Clean(internal.Config.CacheDir))
	if err != nil {
		return err
	}

	var (
		wg      sync.WaitGroup
		errChan = make(chan error, len(fHash))
	)

	if len(args) > 1 && len(fHash) > 1 {
		return errors.New("when removing hashes, it is required to only specify one layer")
	}

	for _, hash := range fHash {
		wg.Add(1)
		go func(errChan chan<- error, target string) {
			defer wg.Done()
			delete_tracker := percent.NewIncrementTracker(&progress.Tracker{Message: "Deleting hash", Total: int64(100), Units: progress.UnitsDefault}, 1)
			defer delete_tracker.Tracker.MarkAsDone()
			pw.AppendTracker(delete_tracker.Tracker)

			if len(fHash) > 0 {
				err := os.Remove(path.Join(cache_dir, args[0], target))
				if err != nil {
					errChan <- err
					return
				}
				return
			} else {
				err := os.RemoveAll(path.Join(cache_dir, target))
				if err != nil {
					errChan <- err
					return
				}
			}

			deactivated_layer := path.Join(internal.Config.ExtensionsDir, target) + internal.ValidSysextExtension
			if !fileio.FileExist(deactivated_layer) {
				return
			}

			err = os.Remove(deactivated_layer)
			if err != nil {
				errChan <- err
				return
			}
		}(errChan, hash)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		slog.Warn(fmt.Sprintf("Error encountered when deleting targets: %s", err.Error()), slog.String("error", err.Error()))
	}

	if len(errChan) == 0 {
		slog.Info("Successfully deleted target from cache", slog.String("hashes", strings.Join(fHash, " ")))
	}

	return nil
}
