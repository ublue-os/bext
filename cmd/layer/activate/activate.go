package activate

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/ublue-os/bext/internal"
)

var ActivateCmd = &cobra.Command{
	Use:   "activate [TARGET...]",
	Short: "Activate layers and refresh sysext",
	Long:  `Activate selected layers and refresh the system extensions store.`,
	RunE:  activateCmd,
	Args:  cobra.MinimumNArgs(1),
}

var (
	fFromFile bool
	fOverride bool
)

func init() {
	ActivateCmd.Flags().BoolVarP(&fFromFile, "file", "f", false, "Parse positional arguments as files instead of layers")
	ActivateCmd.Flags().BoolVar(&fOverride, "override", true, "Write over old symlinks")
}

func activateCmd(cmd *cobra.Command, args []string) error {
	extensions_dir, err := filepath.Abs(path.Clean(internal.Config.ExtensionsDir))
	if err != nil {
		return err
	}

	var (
		errChan = make(chan error, len(args))
		wg      sync.WaitGroup
	)
	cache_dir, err := filepath.Abs(path.Clean(internal.Config.CacheDir))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(internal.Config.ExtensionsDir, 0755); err != nil {
		return err
	}

	for _, target_file := range args {
		slog.Debug("Activating layer "+target_file,
			slog.Bool("fromfile", fFromFile),
			slog.String("layer", target_file),
		)

		wg.Add(1)
		go func(errChan chan<- error, target string) {
			defer wg.Done()
			var (
				deployment_path string
				target_path     string
			)

			if !strings.HasSuffix(target, internal.ValidSysextExtension) && fFromFile {
				errChan <- errors.New("failed to parse file name, invalid sysext extension. should be " + internal.ValidSysextExtension)
				return
			}

			if fFromFile {
				layer_name := strings.Split(path.Base(target), ".")[0]
				target_path = path.Join(extensions_dir, layer_name)
				deployment_path, err = filepath.Abs(layer_name)
				if err != nil {
					errChan <- err
					return
				}
			} else {
				deployment_path = path.Join(cache_dir, target, internal.CurrentBlobName)
				if _, err := os.Stat(deployment_path); err != nil {
					errChan <- errors.New("target layer " + target + " could not be found")
					return
				}
				target_path = path.Join(extensions_dir, target+internal.ValidSysextExtension)
			}
			if fOverride {
				_ = os.Remove(target_path)
			} else {
				errChan <- errors.New(target + " is already activated")
			}
			if err := os.Symlink(deployment_path, target_path); err != nil {
				errChan <- err
			}
		}(errChan, target_file)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		slog.Warn(fmt.Sprintf("Error encountered when activating layers: %s", err.Error()), slog.String("error", err.Error()))
	}

	if len(errChan) == 0 {
		slog.Info("Successfully activated layers", slog.String("layers", strings.Join(args, " ")))
	}
	return nil
}
