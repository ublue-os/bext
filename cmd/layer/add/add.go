package add

import (
	"crypto/md5"
	"encoding/hex"
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
	"github.com/ublue-os/bext/pkg/filecomp"
	"github.com/ublue-os/bext/pkg/fileio"

	"github.com/ublue-os/bext/pkg/logging"
	percent "github.com/ublue-os/bext/pkg/percentmanager"
)

var AddCmd = &cobra.Command{
	Use:   "add [TARGET...]",
	Short: "Add a built layer onto the cache and activate it",
	Long:  `Copy TARGET over to cache-dir as a blob with the TARGET's sha256 as the filename`,
	RunE:  addCmd,
	Args:  cobra.MinimumNArgs(1),
}

var (
	fNoSymlink  bool
	fNoChecksum bool
	fOverride   bool
)

func init() {
	AddCmd.Flags().BoolVar(&fNoSymlink, "no-symlink", false, "Do not activate layer once added to cache")
	AddCmd.Flags().BoolVar(&fNoChecksum, "no-checksum", false, "Do not check if layer was properly added to cache")
	AddCmd.Flags().BoolVar(&fOverride, "override", false, "Override blob if they are already written to cache")
}

func CheckBlobIntegrity(expectedSum []byte, target string) (bool, error) {
	var written_file *os.File
	written_file, err := os.Open(target)
	if err != nil {
		return false, err
	}
	defer written_file.Close()

	return filecomp.CheckExpectedSum(md5.New(), expectedSum, written_file)
}

func addCmd(cmd *cobra.Command, args []string) error {
	pw := percent.NewProgressWriter()
	if !*internal.Config.NoProgress {
		go pw.Render()
		slog.SetDefault(logging.NewMuteLogger())
	}
	pw.SetNumTrackersExpected(len(args))

	if err := os.MkdirAll(internal.Config.CacheDir, 0755); err != nil {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(args))

	for _, layer := range args {
		wg.Add(1)
		go func(layer string, errorChan chan<- error) {
			defer wg.Done()
			target_layer := &internal.TargetLayerInfo{}
			target_layer.Path = path.Clean(layer)

			var err error
			target_layer.FileInfo, err = os.Stat(target_layer.Path)
			if err != nil {
				errChan <- err
				return
			}

			var expectedSections int = 4

			if !fNoSymlink {
				expectedSections++
			}
			if !fNoChecksum {
				expectedSections += 2
			}

			add_tracker := percent.NewIncrementTracker(&progress.Tracker{Message: "Adding layer", Total: target_layer.FileInfo.Size(), Units: progress.UnitsBytes}, expectedSections)
			pw.AppendTracker(add_tracker.Tracker)

			fileContent, err := os.ReadFile(target_layer.Path)
			if err != nil {
				errChan <- err
				return
			}
			target_layer.Data = fileContent
			target_layer.LayerName = strings.Split(path.Base(target_layer.Path), ".")[0]
			layer_sha := md5.New()
			layer_sha.Write(target_layer.Data)
			target_layer.UUID = layer_sha.Sum(nil)
			if err != nil {
				errChan <- err
				return
			}
			blob_filepath, err := filepath.Abs(path.Join(internal.Config.CacheDir, target_layer.LayerName, hex.EncodeToString(target_layer.UUID)))
			if err != nil {
				add_tracker.Tracker.MarkAsErrored()
				errChan <- err
				return
			}

			add_tracker.IncrementSection()
			if err := os.MkdirAll(path.Dir(blob_filepath), 0755); err != nil {
				add_tracker.Tracker.MarkAsErrored()
				errChan <- err
				return
			}

			if fileio.FileExist(blob_filepath) && !fOverride {
				add_tracker.Tracker.MarkAsErrored()
				errChan <- errors.New("Blob " + path.Base(blob_filepath) + " is already in cache")
				return
			}

			add_tracker.IncrementSection()
			slog.Warn(fmt.Sprintf("Copying blob %s %s", target_layer.Path, blob_filepath))
			if err := fileio.FileCopy(target_layer.Path, blob_filepath); err != nil {
				errChan <- err
				return
			}

			if !fNoChecksum {
				add_tracker.Tracker.Message = "Checking blob"

				add_tracker.IncrementSection()
				integrity, err := CheckBlobIntegrity(target_layer.UUID, blob_filepath)

				if err != nil || !integrity {
					add_tracker.Tracker.MarkAsErrored()
					errChan <- fmt.Errorf("copied blobs did not match. source: %s ; target: %s", target_layer.Path, blob_filepath)
					return
				}
			}

			if fNoSymlink {
				add_tracker.Tracker.MarkAsDone()
				return
			}

			var current_blob_path string
			current_blob_path, err = filepath.Abs(path.Join(path.Dir(blob_filepath), internal.CurrentBlobName))
			if err != nil {
				errChan <- err
				return
			}
			slog.Debug("Refreshing symlink", slog.String("path", current_blob_path))
			add_tracker.IncrementSection()
			if _, err := os.Lstat(current_blob_path); err == nil {
				err = os.Remove(current_blob_path)
				if err != nil {
					add_tracker.Tracker.MarkAsErrored()
					errChan <- err
					return
				}
			} else if errors.Is(err, os.ErrNotExist) {

			} else {
				add_tracker.Tracker.MarkAsErrored()
				errChan <- err
				return
			}

			err = os.Symlink(blob_filepath, current_blob_path)
			if err != nil {
				add_tracker.Tracker.MarkAsErrored()
				errChan <- err
				return
			}
			add_tracker.Tracker.MarkAsDone()
		}(layer, errChan)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		slog.Warn(fmt.Sprintf("Error encountered when adding blobs: %s", err.Error()), slog.String("error", err.Error()))
	}

	if len(errChan) == 0 {
		slog.Info("Successfully added blobs to cache", slog.String("blobs", strings.Join(args, " ")))
	}
	return nil
}
