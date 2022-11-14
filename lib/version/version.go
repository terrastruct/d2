package version

import (
	"context"
	"fmt"

	"github.com/google/go-github/github"
	"oss.terrastruct.com/cmdlog"
)

// Pre-built binaries will have version set during build time.
var Version = "master (built from source)"

func CheckVersion(ctx context.Context, logger *cmdlog.Logger) {
	fmt.Println(Version)

	if Version == "master (built from source)" {
		return
	}

	// Install script uses -v to check the version, we shouldn't be checking for
	// updates here...
	// https://github.com/terrastruct/d2/issues/49#issuecomment-1313229683
	return

	logger.Info.Printf("Checking for updates...")
	latest, err := getLatestVersion(ctx)
	if err != nil {
		logger.Debug.Printf("Error reaching Github for latest version: %s", err.Error())
	} else if Version != "master" && Version != latest {
		logger.Info.Printf("A new version of D2 is available: %s -> %s", Version, latest)
	}
}

func getLatestVersion(ctx context.Context) (string, error) {
	client := github.NewClient(nil)
	rep, _, err := client.Repositories.GetLatestRelease(ctx, "terrastruct", "d2")

	if err != nil {
		return "", err
	}

	return *rep.TagName, nil
}
