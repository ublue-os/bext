package deactivate

import (
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

var DeactivateCmd = &cobra.Command{
	Use:   "deactivate [TARGET...]",
	Short: "Deactivate a layer and refresh sysext",
	Long:  `Deativate a selected layer (unsymlink it from /var/lib/extensions) and refresh the system extensions store.`,
	RunE:  deactivateCmd,
	Args:  cobra.MinimumNArgs(1),
}

func deactivateCmd(cmd *cobra.Command, args []string) error {
	extensions_dir, err := filepath.Abs(path.Clean(internal.Config.ExtensionsDir))
	if err != nil {
		return err
	}

	var (
		errChan chan error
		wg      sync.WaitGroup
	)

	for _, target_layer := range args {
		wg.Add(1)
		go func(errChan chan<- error, target string) {
			defer wg.Done()

			if err := os.Remove(path.Join(extensions_dir, target+internal.ValidSysextExtension)); err != nil {
				errChan <- err
				return
			}
		}(errChan, target_layer)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		slog.Warn(fmt.Sprintf("Error encountered when deactivating layers: %s", err.Error()), slog.String("error", err.Error()))
	}

	if len(errChan) == 0 {
		slog.Info("Successfully deactivated layers", slog.String("layers", strings.Join(args, " ")))
	}

	return nil
}
