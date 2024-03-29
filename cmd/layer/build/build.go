package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/containers/podman/v4/libpod/define"
	"github.com/containers/podman/v4/pkg/bindings"
	"github.com/containers/podman/v4/pkg/bindings/containers"
	"github.com/containers/podman/v4/pkg/bindings/images"
	"github.com/containers/podman/v4/pkg/specgen"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/spf13/cobra"
	"github.com/ublue-os/bext/internal"
	"github.com/ublue-os/bext/pkg/logging"
	"github.com/ublue-os/bext/pkg/percentmanager"
)

var BuildCmd = &cobra.Command{
	Use:   "build [CONFIG]",
	Short: "Build an image from a configuration file",
	Long:  `Build an image from a configuration file`,
	RunE:  buildCmd,
}

var (
	fNixosImage        *string
	fNixosImageTag     *string
	fRecipeMakerFlake  *string
	fRecipeMakerAction *string
	fOutputPath        *string
	fNoPull            *bool
	fKeep              *bool
)

func init() {
	fNixosImage = BuildCmd.Flags().StringP("image", "i", "docker.io/nixos/nix", "Image that will be used for building the nix image")
	fNixosImageTag = BuildCmd.Flags().StringP("tag", "t", "latest", "Image tag used for the building container")
	fRecipeMakerFlake = BuildCmd.Flags().StringP("recipe-flake", "r", "github:ublue-os/bext", "Nix flake that will be used as base for building the image")
	fRecipeMakerAction = BuildCmd.Flags().StringP("recipe-action", "a", "bake-recipe", "Derivation that will be built on recipe-flake")
	fOutputPath = BuildCmd.Flags().StringP("output-path", "o", "", "Path of the file for the image")
	fNoPull = BuildCmd.Flags().Bool("no-pull", false, "Do not pull the nix image even if conditions are met")
	fKeep = BuildCmd.Flags().Bool("keep", false, "Keep the build containers instead of getting rid of them (Mostly for debugging issues)")
}

func buildCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return internal.NewPositionalError("CONFIG")
	}

	pw := percent.NewProgressWriter()

	var (
		expectedSections = 6
		expectedTrackers = 1
	)

	if *fKeep {
		expectedSections--
	}
	if !*fNoPull {
		expectedTrackers++
	}

	pw.SetNumTrackersExpected(expectedTrackers)
	build_tracker := percent.NewIncrementTracker(&progress.Tracker{
		Message: "Building image",
		Total:   int64(100),
		Units:   progress.UnitsDefault},
		expectedSections)

	if !*internal.Config.NoProgress {
		go pw.Render()
		slog.SetDefault(logging.NewMuteLogger())
	}
	pw.AppendTracker(build_tracker.Tracker)

	config_file_path, err := filepath.Abs(path.Clean(args[0]))
	if err != nil {
		return err
	}
	config_data, err := os.ReadFile(config_file_path)
	if err != nil {
		return err
	}

	configuration := &internal.LayerConfiguration{}

	err = json.Unmarshal(config_data, configuration)
	if err != nil {
		return err
	}

	sock_dir := os.Getenv("XDG_RUNTIME_DIR")
	if sock_dir == "" {
		sock_dir = "/var/run"
	}
	socket := "unix:" + sock_dir + "/podman/podman.sock"

	conn, err := bindings.NewConnection(context.Background(), socket)
	if err != nil {
		slog.Warn("A podman socket is required, enable it with \"systemctl enable --now --user podman.socket\"")
		return err
	}
	build_tracker.IncrementSection()

	full_image_name := *fNixosImage + ":" + *fNixosImageTag

	if !*fNoPull {
		image_summary, err := images.List(conn, &images.ListOptions{All: &[]bool{true}[0]})
		if err != nil {
			return err
		}

		var already_has_image = false
		for _, image := range image_summary {
			if slices.Contains(image.History, full_image_name) {
				already_has_image = true
			}
		}

		if !already_has_image {
			slog.Info("Pulling image", slog.String("image name", full_image_name))
			tracker := progress.Tracker{Message: "Pulling image", Total: int64(100), Units: progress.UnitsDefault}
			pw.AppendTracker(&tracker)

			var pull_opt = &images.PullOptions{ProgressWriter: &io.Discard}
			if *internal.Config.NoProgress {
				pull_opt = &images.PullOptions{}
			}

			tracker.Increment(0)
			if _, err := images.Pull(conn, full_image_name, pull_opt); err != nil {
				return err
			}
			tracker.Increment(100)
		}
	}
	build_tracker.IncrementSection()

	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	var out_path string
	if *fOutputPath != "" {
		out_path, err = filepath.Abs(path.Clean(*fOutputPath))
		if err != nil {
			build_tracker.Tracker.MarkAsErrored()
			return err
		}
	} else {
		out_path, err = filepath.Abs(path.Join(pwd, configuration.Name+internal.ValidSysextExtension))
		if err != nil {
			build_tracker.Tracker.MarkAsErrored()
			return err
		}
	}

	nix_flags := "-L --extra-experimental-features nix-command --extra-experimental-features flakes --impure"

	spec := specgen.NewSpecGenerator(full_image_name, false)
	spec.Mounts = append(spec.Mounts, specs.Mount{
		Source:      path.Dir(out_path),
		Destination: "/out",
		Type:        define.TypeBind,
		Options:     []string{"Z", "rw"},
	})
	spec.Mounts = append(spec.Mounts, specs.Mount{
		Source:      config_file_path,
		Destination: "/config.json",
		Type:        define.TypeBind,
		Options:     []string{"Z", "ro"},
	})

	build_tracker.IncrementSection()
	spec.Env = map[string]string{"BEXT_CONFIG_FILE": "/config.json"}
	spec.WorkDir = "/out"

	container_command := fmt.Sprintf(`set -eux ; NIXPKGS_ALLOW_UNFREE=1 nix build %s %s#%s -o result && cp -f ./result ./%s && rm ./result`, nix_flags, *fRecipeMakerFlake, *fRecipeMakerAction, path.Base(out_path))

	spec.Command = []string{"/bin/sh", "-c", container_command}
	createResponse, err := containers.CreateWithSpec(conn, spec, nil)
	if err != nil {
		build_tracker.Tracker.MarkAsErrored()
		return err
	}

	slog.Info("Starting build container", slog.String("containerID", createResponse.ID))
	build_tracker.IncrementSection()
	if err := containers.Start(conn, createResponse.ID, nil); err != nil {
		build_tracker.Tracker.MarkAsErrored()
		return err
	}

	slog.Info("Waiting for container response", slog.String("containerID", createResponse.ID))
	build_tracker.IncrementSection()
	if _, err := containers.Wait(conn, createResponse.ID, nil); err != nil {
		build_tracker.Tracker.MarkAsErrored()
		return err
	}

	build_tracker.IncrementSection()
	if !*fKeep {
		slog.Debug("Deleting build container", slog.String("containerID", createResponse.ID))
		if _, err := containers.Remove(conn, createResponse.ID, nil); err != nil {
			build_tracker.Tracker.MarkAsErrored()
			return err
		}
		build_tracker.Tracker.MarkAsDone()
	}
	build_tracker.Tracker.MarkAsDone()

	slog.Info(fmt.Sprintf("Successfully built %s", path.Base(out_path)), slog.String("containerId", createResponse.ID), slog.String("imagename", out_path))

	return nil
}
