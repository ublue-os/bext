package cmd

import (
	"log/slog"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/ublue-os/bext/cmd/layer"
	"github.com/ublue-os/bext/cmd/mount"
	"github.com/ublue-os/bext/internal"
	"github.com/ublue-os/bext/pkg/logging"
	appLogging "github.com/ublue-os/bext/pkg/logging"
)

var RootCmd = &cobra.Command{
	Use:               "bext",
	Short:             "Manager for Systemd system extensions",
	Long:              `Manage your systemd system extensions from your CLI, managing their cache, multiple versions, and building.`,
	PersistentPreRunE: initLogging,
	SilenceUsage:      true,
}

var (
	fLogFile   string
	fLogLevel  string
	fNoLogging bool
)

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		slog.Debug("Application exited with error", slog.String("errormsg", err.Error()), slog.Int("exitcode", 1))
		os.Exit(1)
	}
}

func initLogging(cmd *cobra.Command, args []string) error {
	var logWriter *os.File = os.Stdout
	if fLogFile != "-" {
		abs, err := filepath.Abs(path.Clean(fLogFile))
		if err != nil {
			return err
		}
		logWriter, err = os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0755)
		if err != nil {
			return err
		}
	}

	logLevel, err := appLogging.StrToLogLevel(fLogLevel)
	if err != nil {
		return err
	}

	main_app_logger := slog.New(appLogging.SetupAppLogger(logWriter, logLevel, fLogFile != "-"))

	if fNoLogging {
		slog.SetDefault(logging.NewMuteLogger())
	} else {
		slog.SetDefault(main_app_logger)
	}
	return nil
}

func init() {
	RootCmd.PersistentFlags().StringVar(&fLogFile, "log-file", "-", "File where user-facing logs will be written to")
	RootCmd.PersistentFlags().StringVar(&fLogLevel, "log-level", "info", "Log level for user-facing logs")
	RootCmd.PersistentFlags().BoolVar(&fNoLogging, "quiet", false, "Do not log anything to anywhere")
	internal.Config.NoProgress = RootCmd.PersistentFlags().Bool("no-progress", false, "Do not use progress bars whenever they would be")

	RootCmd.AddCommand(layer.LayerCmd)
	RootCmd.AddCommand(mount.MountCmd)
	RootCmd.AddCommand(AddToPathCmd)
}
