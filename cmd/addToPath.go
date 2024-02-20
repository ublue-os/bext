package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ublue-os/bext/internal"
	"github.com/ublue-os/bext/pkg/fileio"
)

var AddToPathCmd = &cobra.Command{
	Use:   "add-to-path [...SHELL]",
	Short: "Add the mounted layer binaries to your path",
	Long:  `Write a snippet for your shell of the mounted path for the activated bext layers`,
	RunE:  addToPathCmd,
}

type ShellDefinition struct {
	Snippet string
	RcPath  string
}

var (
	fPathPath *string
	fRCPath   *string
)

func init() {
	fPathPath = AddToPathCmd.Flags().StringP("path", "p", "/tmp/extensions.d/bin", "Path where all shared binaries are being mounted to")
	fRCPath = AddToPathCmd.Flags().StringP("rc-path", "r", "", "RC path for your chosen shell instead of the default")
}

func addToPathCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return internal.NewPositionalError("SHELL...")
	}
	if len(args) > 0 && *fRCPath != "" {
		//TODO: make this be possible, honestly, there should be something like zip() to sync up those.
		slog.Warn("Cannot write multiple shell snippets with rc path being specified, will write to default paths")
		os.Exit(1)
	}

	user_home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	user_config, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	var defaultValues = map[string]ShellDefinition{
		"bash": {
			RcPath:  fmt.Sprintf("%s/.bashrc", user_home),
			Snippet: fmt.Sprintf("[ -e %s ] && PATH=\"$PATH:%s\" \n", *fPathPath, *fPathPath),
		},
		"zsh": {
			RcPath:  fmt.Sprintf("%s/.zshrc", user_home),
			Snippet: fmt.Sprintf("[ -e %s ] && PATH=\"$PATH:%s\" \n", *fPathPath, *fPathPath),
		},
		"nu": {
			RcPath:  fmt.Sprintf("%s/config.nu", user_config),
			Snippet: fmt.Sprintf("$env.PATH = ($env.PATH | split row (char esep) | append %s)\n", *fPathPath),
		},
	}

	var valid_stuff []string
	for key := range defaultValues {
		valid_stuff = append(valid_stuff, key)
	}

	for _, shell := range args {
		cleaned_shell := path.Base(path.Clean(shell))

		if !slices.Contains(valid_stuff, cleaned_shell) {
			slog.Warn(fmt.Sprintf("Could not find shell %s, valid shells are: %s", cleaned_shell, strings.Join(valid_stuff, ", ")))
			os.Exit(1)
		}

		var rcPath string
		if *fRCPath != "" {
			rcPath = path.Clean(*fRCPath)
		} else {
			rcPath = defaultValues[cleaned_shell].RcPath
		}

		if _, err := fileio.FileAppendS(rcPath, defaultValues[cleaned_shell].Snippet); err != nil {
			slog.Warn(fmt.Sprintf("Failed writing %s snippet to %s", cleaned_shell, rcPath), slog.String("source", cleaned_shell), slog.String("target", rcPath))
			return err
		}
		slog.Info(fmt.Sprintf("Successfully written snippet to %s", rcPath))
	}

	return nil
}
