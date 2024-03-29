package path

import (
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/ublue-os/bext/internal"
)

var PathCmd = &cobra.Command{
	Use:   "path",
	Short: "Mount all /bin paths from each layer to target destination",
	Long:  `Mount all /bin paths from each layer to target destination`,
	RunE:  pathCmd,
}

var (
	fPathPath *string
)

func init() {
	fPathPath = PathCmd.Flags().StringP("path", "p", "/tmp/extensions.d/bin", "Path where all shared binaries will be mounted to")
}

func pathCmd(cmd *cobra.Command, args []string) error {
	path_path, err := filepath.Abs(path.Clean(*fPathPath))
	if err != nil {
		return err
	}

	if *internal.Config.UnmountFlag {
		slog.Debug("Unmounting", slog.String("target", path_path))
		if err := syscall.Unmount(path_path, 0); err != nil {
			slog.Warn("Failed unmounting path", slog.String("target", path_path))
			return err
		}
		slog.Info("Successfuly unmounted path "+path_path, slog.String("path", path_path))
		return nil
	}

	extensions_mount, err := filepath.Abs(path.Clean(internal.Config.ExtensionsMount))
	if err != nil {
		return nil
	}

	layers, err := os.ReadDir(extensions_mount)
	if err != nil {
		slog.Warn("No layers are mounted")
		return err
	}

	var valid_layers []string
	for _, layer := range layers {
		if _, err := os.Stat(path.Join(extensions_mount, layer.Name(), "bin")); err != nil {
			continue
		}
		valid_layers = append(valid_layers, layer.Name())
	}

	if err := os.MkdirAll(path_path, 0755); err != nil {
		slog.Warn("Failed creating mount path", slog.String("target", path_path))
		return err
	}

	if len(layers) == 0 {
		slog.Warn("No valid layers are mounted")
		os.Exit(1)
	} else if len(layers) == 1 {
		mount_path := path.Join(extensions_mount, layers[0].Name(), "bin")
		if _, err := os.Stat(path_path); err == nil {
			slog.Debug("Unmounting", slog.String("path", path_path))
			_ = syscall.Unmount(path_path, 0)
		}

		if err := syscall.Mount(mount_path, path_path, "bind", uintptr(syscall.MS_BIND|syscall.MS_RDONLY), ""); err != nil {
			slog.Warn("Failed mounting bindmount to path", slog.String("source", mount_path), slog.String("target", path_path))
			return err
		}
	} else {
		slog.Debug("Unmounting", slog.String("path", path_path))
		_ = syscall.Unmount(path_path, 0)

		slog.Debug("Mounting path with overlayFS", slog.String("layers", strings.Join(valid_layers, " ")), slog.String("target", path_path))
		err = syscall.Mount("none", path_path, "overlayfs", uintptr(syscall.MS_RDONLY|syscall.MS_NODEV|syscall.MS_NOATIME), "lowerdir="+strings.Join(valid_layers, ":"))
		if err != nil {
			return err
		}
	}

	slog.Info("Successfully mounted PATH")

	return nil
}
