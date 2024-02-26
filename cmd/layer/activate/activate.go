package activate

import (
	"errors"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

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

var fFromFile bool

func init() {
	ActivateCmd.Flags().BoolVarP(&fFromFile, "file", "f", false, "Parse positional arguments as files instead of layers")
}

func activateCmd(cmd *cobra.Command, args []string) error {
	extensions_dir, err := filepath.Abs(path.Clean(internal.Config.ExtensionsDir))
	if err != nil {
		return err
	}

	if fFromFile {
		for _, target_file := range args {
			if !strings.HasSuffix(target_file, internal.ValidSysextExtension) {
				return errors.New("failed to parse file name, invalid sysext extension. Should be " + internal.ValidSysextExtension)
			}

			deployment_path := path.Join(extensions_dir, path.Base(target_file))
			slog.Debug("Activating layer "+target_file,
				slog.Bool("fromfile", fFromFile),
				slog.String("file layer", target_file),
				slog.String("path", deployment_path),
			)

			file_abs, err := filepath.Abs(path.Clean(target_file))
			if err != nil {
				return err
			}

			_ = os.Remove(file_abs)

			if err := os.Symlink(file_abs, path.Join(extensions_dir, path.Base(file_abs))); err != nil {
				return err
			}
			slog.Info("Successfully activated layer " + path.Base(file_abs))
		}

		return nil
	}

	cache_dir, err := filepath.Abs(path.Clean(internal.Config.CacheDir))
	if err != nil {
		return err
	}

	for _, target_layer := range args {
		current_blob_path := path.Join(cache_dir, target_layer, internal.CurrentBlobName)
		if _, err := os.Stat(current_blob_path); err != nil {
			return errors.New("target layer " + target_layer + " could not be found")
		}

		if err := os.MkdirAll(internal.Config.ExtensionsDir, 0755); err != nil {
			return err
		}

		target_path := path.Join(extensions_dir, path.Base(path.Dir(current_blob_path))+internal.ValidSysextExtension)
		slog.Debug("Activating layer",
			slog.Bool("fromfile", fFromFile),
			slog.String("layer", target_layer),
			slog.String("blob", current_blob_path),
		)

		_ = os.Remove(target_path)

		if err := os.Symlink(current_blob_path, target_path); err != nil {
			return err
		}

		slog.Info("Successfully activated layer " + path.Base(target_layer))
	}

	return nil
}
