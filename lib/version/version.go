package version

import "regexp"

// Pre-built binaries will have version set correctly during build time.
var Version = "v0.7.1-HEAD"

func OnlyNumbers() string {
	re, err := regexp.Compile("[0-9]+.[0-9]+.[0-9]+")
	if err != nil {
		return ""
	}
	return re.FindString(Version)
}
