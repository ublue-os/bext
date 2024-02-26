package clean

import (
	"errors"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"
	"github.com/ublue-os/bext/internal"
)

var CleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean every unused cache blob",
	Long:  `Clean every unused blob from cache except the current blob and its symlink`,
	RunE:  cleanCmd,
}

var (
	fExclude *[]string
	fDryRun  *bool
)

func init() {
	fExclude = CleanCmd.Flags().StringSliceP("exclude", "e", make([]string, 0), "Exclude directories from cleaning")
	fDryRun = CleanCmd.Flags().Bool("dry-run", false, "Do not actually clean anything, just print what would be deleted")
}

func cleanCmd(cmd *cobra.Command, args []string) error {
	cache_dir, err := filepath.Abs(internal.Config.CacheDir)
	if err != nil {
		return err
	}
	target_cache, err := os.ReadDir(cache_dir)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	for _, entry := range target_cache {
		if !entry.IsDir() {
			continue
		}
		slog.Info("Cleaning layer " + entry.Name())

		entry_dir_path := path.Join(cache_dir, entry.Name())
		entry_dir, err := os.ReadDir(entry_dir_path)
		if err != nil {
			return err
		}

		if len(entry_dir) < 1 {
			wg.Add(1)
			go func() {
				wg.Done()
				os.Remove(entry_dir_path)
			}()
			continue
		}

		var do_not_clean map[string]bool = make(map[string]bool)

		for _, provided_path := range *fExclude {
			managed_path, err := filepath.Abs(path.Clean(provided_path))
			if err != nil {
				return err
			}
			do_not_clean[managed_path] = true
		}

		for _, cache_blob := range entry_dir {
			if cache_blob.IsDir() {
				continue
			}

			cleanpath := path.Join(entry_dir_path, cache_blob.Name())

			fstat, err := os.Lstat(cleanpath)
			if err != nil {
				return err
			}

			if fstat.Mode().Type() == os.ModeSymlink && fstat.Name() == internal.CurrentBlobName {
				eval_link, err := filepath.EvalSymlinks(cleanpath)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				} else if errors.Is(err, os.ErrNotExist) {
					continue
				}
				do_not_clean[eval_link] = true
				do_not_clean[cleanpath] = true
				continue
			}
			if _, exists := do_not_clean[cleanpath]; exists || *fDryRun {
				continue
			}

			slog.Debug("Cleaned path", slog.String("path", cleanpath))
			wg.Add(1)
			go func() {
				defer wg.Done()
				os.Remove(cleanpath)
			}()
		}
	}
	wg.Wait()

	return nil
}
