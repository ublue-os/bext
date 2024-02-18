package store

import (
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/ublue-os/bext/internal"
	"github.com/ublue-os/bext/pkg/chattr"
)

var StoreCmd = &cobra.Command{
	Use:   "store",
	Short: fmt.Sprintf("Mount %s to %s safely", LayeredStorePath, NixStorePath),
	Long:  fmt.Sprintf(`Mount %s to %s so that your layered binaries may work`, LayeredStorePath, NixStorePath),
	RunE:  storeCmd,
}

const NixStorePath = "/nix/store"
const LayeredStorePath = "/usr/store"

var (
	fStoreBindmountPath *string
)

func init() {
	fStoreBindmountPath = StoreCmd.Flags().String("bindmount-path", "/tmp/nix-store-bindmount", "Path where an already existing nix store will be bind-mounted to")
}

func storeCmd(cmd *cobra.Command, args []string) error {
	bindmount_path, err := filepath.Abs(path.Clean(*fStoreBindmountPath))
	if err != nil {
		return err
	}

	if *internal.Config.UnmountFlag {
		slog.Debug("Unmounting store", slog.String("target", NixStorePath))
		if err := syscall.Unmount(NixStorePath, 0); err != nil {
			return err
		}

		slog.Debug("Unmounting bindmount", slog.String("target", bindmount_path))
		if err := syscall.Unmount(bindmount_path, 0); err != nil {
			return err
		}
		slog.Info("Successfully unmounted store and bindmount", slog.String("store_path", NixStorePath), slog.String("bindmount_path", bindmount_path))
		return nil
	}

	if _, err := os.Stat("/nix"); err != nil {
		root_dir, err := os.Open("/")
		if err != nil {
			return err
		}
		defer root_dir.Close()

		slog.Debug("Creating nix store", slog.String("target", NixStorePath))
		err = chattr.SetAttr(root_dir, chattr.FS_IMMUTABLE_FL)
		if err != nil {
			slog.Warn("Failed unsetting immutable attributes to /", slog.String("target", "/"))
			return err
		}
		if err := os.MkdirAll(NixStorePath, 0755); err != nil {
			slog.Warn("Failed creating root nix store path", slog.String("target", NixStorePath))
			return err
		}
		err = chattr.UnsetAttr(root_dir, chattr.FS_IMMUTABLE_FL)
		if err != nil {
			slog.Warn("Failed setting immutable attributes to /", slog.String("target", "/"))
			return err
		}
	} else if _, err := os.Stat(NixStorePath); err != nil {
		if err := os.MkdirAll(NixStorePath, 0755); err != nil {
			slog.Warn("Failed creating root nix store path", slog.String("target", NixStorePath))
			return err
		}
	}

	store_contents, err := os.ReadDir(NixStorePath)
	if err != nil {
		return err
	}

	if len(store_contents) > 0 {
		_ = syscall.Unmount(NixStorePath, 0)
	}

	if _, err := os.Stat(bindmount_path); err != nil && len(store_contents) > 0 {
		slog.Debug("Creating bindmount", slog.String("target", bindmount_path))
		if err := os.MkdirAll(bindmount_path, 0755); err != nil {
			return err
		}

		_ = syscall.Unmount(NixStorePath, 0)
		_ = syscall.Unmount(bindmount_path, 0)

		slog.Debug("Mounting bindmount", slog.String("source", bindmount_path), slog.String("target", NixStorePath))
		if err := syscall.Mount(NixStorePath, bindmount_path, "bind", uintptr(syscall.MS_BIND), ""); err != nil {
			slog.Warn("Failed mounting root nix store to bindmount", slog.String("source", NixStorePath), slog.String("target", bindmount_path))
			return err
		}

		slog.Debug("Mounting store to itself", slog.String("source", NixStorePath), slog.String("target", bindmount_path))
		if err := syscall.Mount(bindmount_path, NixStorePath, "bind", uintptr(syscall.MS_BIND), ""); err != nil {
			slog.Warn("Failed mounting bindmount to root nix store", slog.String("source", bindmount_path), slog.String("target", NixStorePath))
			return err
		}
	}

	if _, err := os.Stat(LayeredStorePath); err != nil {
		slog.Warn("No layered store could be found in " + LayeredStorePath)
		return err
	}

	slog.Info(fmt.Sprintf("Mounting %s to %s", LayeredStorePath, NixStorePath), slog.String("source", LayeredStorePath), slog.String("target", NixStorePath))
	if err := syscall.Mount(LayeredStorePath, NixStorePath, "bind", uintptr(syscall.MS_BIND|syscall.MS_RDONLY), ""); err != nil {
		slog.Warn("Failed mounting layered nix stores to root nix store.", slog.String("source", LayeredStorePath), slog.String("target", NixStorePath))
		return err
	}
	return nil
}
