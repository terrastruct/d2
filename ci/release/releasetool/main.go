package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

const mainModule = "github.com/d2lang/d2"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "release-tool:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected archive, checksums, sbom, verify-checksums, verify-binary, or verify-archive")
	}
	switch args[0] {
	case "archive":
		return archiveCommand(args[1:])
	case "checksums":
		return checksumsCommand(args[1:])
	case "sbom":
		return sbomCommand(args[1:])
	case "verify-checksums":
		return verifyChecksumsCommand(args[1:])
	case "verify-binary":
		return verifyBinaryCommand(args[1:])
	case "verify-archive":
		return verifyArchiveCommand(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func sbomCommand(args []string) error {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	binary := fs.String("binary", "", "release binary to inspect")
	output := fs.String("output", "", "output SPDX JSON path")
	name := fs.String("name", "", "document name")
	version := fs.String("version", "", "release version")
	epoch := fs.Int64("epoch", -1, "creation Unix timestamp")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *binary == "" || *output == "" || *name == "" || *version == "" || *epoch < 0 {
		return errors.New("sbom requires --binary, --output, --name, --version, and a non-negative --epoch")
	}
	return writeSBOM(*binary, *output, *name, *version, time.Unix(*epoch, 0).UTC())
}

func archiveCommand(args []string) error {
	fs := flag.NewFlagSet("archive", flag.ContinueOnError)
	root := fs.String("root", "", "release directory to archive")
	output := fs.String("output", "", "output .tar.gz path")
	epoch := fs.Int64("epoch", -1, "normalized Unix timestamp")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *root == "" || *output == "" || *epoch < 0 {
		return errors.New("archive requires --root, --output, and a non-negative --epoch")
	}
	return writeArchive(*root, *output, time.Unix(*epoch, 0).UTC())
}

func checksumsCommand(args []string) error {
	fs := flag.NewFlagSet("checksums", flag.ContinueOnError)
	output := fs.String("output", "", "output checksum manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output == "" || fs.NArg() == 0 {
		return errors.New("checksums requires --output and at least one file")
	}
	return writeChecksums(*output, fs.Args())
}

func verifyChecksumsCommand(args []string) error {
	fs := flag.NewFlagSet("verify-checksums", flag.ContinueOnError)
	manifest := fs.String("manifest", "", "checksum manifest")
	directory := fs.String("directory", "", "directory containing manifest subjects")
	expectedCount := fs.Int("expected-count", -1, "required number of manifest entries")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *manifest == "" || *directory == "" {
		return errors.New("verify-checksums requires --manifest and --directory")
	}
	return verifyChecksums(*manifest, *directory, *expectedCount)
}

func verifyBinaryCommand(args []string) error {
	fs := flag.NewFlagSet("verify-binary", flag.ContinueOnError)
	binary := fs.String("path", "", "binary path")
	goos := fs.String("goos", "", "expected GOOS")
	goarch := fs.String("goarch", "", "expected GOARCH")
	goVersion := fs.String("go-version", "", "expected Go toolchain version")
	revision := fs.String("revision", "", "expected VCS revision")
	vcsModified := fs.String("vcs-modified", "false", "expected vcs.modified setting")
	maxSize := fs.Int64("max-size", 0, "maximum binary size in bytes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *binary == "" || *goos == "" || *goarch == "" || *goVersion == "" || *revision == "" || *maxSize <= 0 {
		return errors.New("verify-binary requires --path, --goos, --goarch, --go-version, --revision, and a positive --max-size")
	}
	if *vcsModified != "true" && *vcsModified != "false" {
		return errors.New("verify-binary --vcs-modified must be true or false")
	}
	return verifyBinary(*binary, *goos, *goarch, *goVersion, *revision, *vcsModified, *maxSize)
}

func verifyArchiveCommand(args []string) error {
	fs := flag.NewFlagSet("verify-archive", flag.ContinueOnError)
	archive := fs.String("path", "", "archive path")
	root := fs.String("root", "", "expected top-level directory")
	epoch := fs.Int64("epoch", -1, "expected normalized Unix timestamp")
	maxSize := fs.Int64("max-size", 0, "maximum archive size in bytes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *archive == "" || *root == "" || *epoch < 0 || *maxSize <= 0 {
		return errors.New("verify-archive requires --path, --root, a non-negative --epoch, and a positive --max-size")
	}
	return verifyArchive(*archive, *root, time.Unix(*epoch, 0).UTC(), *maxSize)
}

type archiveEntry struct {
	fullPath string
	name     string
	info     fs.FileInfo
	link     string
}

func writeArchive(root, output string, stamp time.Time) (retErr error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("archive root %q is not a directory", root)
	}
	rootName := filepath.Base(root)
	if rootName == "." || rootName == string(filepath.Separator) || rootName == "" {
		return fmt.Errorf("unsafe archive root %q", root)
	}
	if rel, err := filepath.Rel(root, output); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("archive output must not be inside its input tree")
	}

	var entries []archiveEntry
	err = filepath.WalkDir(root, func(fullPath string, dirEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := dirEntry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0 {
			return fmt.Errorf("unsupported file type in release archive: %s", fullPath)
		}
		rel, err := filepath.Rel(filepath.Dir(root), fullPath)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if strings.ContainsAny(name, "\\\x00") {
			return fmt.Errorf("unsafe archive path %q", name)
		}
		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(fullPath)
			if err != nil {
				return err
			}
			link = filepath.ToSlash(link)
			if err := validateSymlink(name, link, rootName); err != nil {
				return err
			}
		}
		entries = append(entries, archiveEntry{fullPath: fullPath, name: name, info: info, link: link})
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(output), "."+filepath.Base(output)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if retErr != nil {
			_ = os.Remove(tmpName)
		}
	}()

	zw, err := gzip.NewWriterLevel(tmp, gzip.BestCompression)
	if err != nil {
		return err
	}
	zw.Header.ModTime = stamp
	zw.Header.OS = 255
	tw := tar.NewWriter(zw)
	for _, entry := range entries {
		header, err := tar.FileInfoHeader(entry.info, entry.link)
		if err != nil {
			return err
		}
		header.Name = entry.name
		header.Uid = 0
		header.Gid = 0
		header.Uname = ""
		header.Gname = ""
		header.ModTime = stamp
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.Format = tar.FormatUSTAR
		switch {
		case entry.info.IsDir():
			header.Name = strings.TrimSuffix(header.Name, "/") + "/"
			header.Mode = 0o755
		case entry.info.Mode()&os.ModeSymlink != 0:
			header.Mode = 0o777
		case entry.info.Mode().IsRegular():
			header.Mode = 0o644
			if entry.info.Mode().Perm()&0o111 != 0 {
				header.Mode = 0o755
			}
		default:
			return fmt.Errorf("unsupported file type in release archive: %s", entry.fullPath)
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !entry.info.Mode().IsRegular() {
			continue
		}
		file, err := os.Open(entry.fullPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Remove(output); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmpName, output); err != nil {
		return err
	}
	return nil
}

func writeChecksums(output string, files []string) error {
	type subject struct {
		path string
		name string
	}
	subjects := make([]subject, 0, len(files))
	seen := map[string]bool{}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("checksum subject %q is not a regular file", file)
		}
		name := filepath.Base(file)
		if seen[name] {
			return fmt.Errorf("duplicate checksum subject name %q", name)
		}
		seen[name] = true
		subjects = append(subjects, subject{path: file, name: name})
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].name < subjects[j].name })

	var manifest strings.Builder
	for _, subject := range subjects {
		digest, err := fileSHA256(subject.path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&manifest, "%s  %s\n", digest, subject.name)
	}
	return writeFileAtomic(output, []byte(manifest.String()), 0o644)
}

type spdxDocument struct {
	SPDXVersion       string             `json:"spdxVersion"`
	DataLicense       string             `json:"dataLicense"`
	SPDXID            string             `json:"SPDXID"`
	Name              string             `json:"name"`
	DocumentNamespace string             `json:"documentNamespace"`
	CreationInfo      spdxCreationInfo   `json:"creationInfo"`
	Packages          []spdxPackage      `json:"packages"`
	Relationships     []spdxRelationship `json:"relationships"`
}

type spdxCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type spdxPackage struct {
	Name               string            `json:"name"`
	SPDXID             string            `json:"SPDXID"`
	VersionInfo        string            `json:"versionInfo,omitempty"`
	DownloadLocation   string            `json:"downloadLocation"`
	FilesAnalyzed      bool              `json:"filesAnalyzed"`
	LicenseConcluded   string            `json:"licenseConcluded"`
	LicenseDeclared    string            `json:"licenseDeclared"`
	CopyrightText      string            `json:"copyrightText"`
	Comment            string            `json:"comment,omitempty"`
	ExternalReferences []spdxExternalRef `json:"externalRefs,omitempty"`
}

type spdxExternalRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

type spdxRelationship struct {
	Element string `json:"spdxElementId"`
	Type    string `json:"relationshipType"`
	Related string `json:"relatedSpdxElement"`
}

func writeSBOM(binaryPath, output, name, version string, stamp time.Time) error {
	build, err := buildinfo.ReadFile(binaryPath)
	if err != nil {
		return err
	}
	document, err := newSPDXDocument(build, name, version, stamp)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(output, data, 0o644)
}

func newSPDXDocument(build *debug.BuildInfo, name, version string, stamp time.Time) (*spdxDocument, error) {
	if build.Path != mainModule {
		return nil, fmt.Errorf("SBOM binary main package is %q, expected %q", build.Path, mainModule)
	}
	revision := ""
	for _, setting := range build.Settings {
		if setting.Key == "vcs.revision" {
			revision = setting.Value
			break
		}
	}
	if revision == "" {
		return nil, errors.New("SBOM binary is missing its VCS revision")
	}

	const noAssertion = "NOASSERTION"
	mainID := "SPDXRef-Package-d2"
	document := &spdxDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              name,
		DocumentNamespace: fmt.Sprintf("https://github.com/d2lang/d2/sbom/%s/%s", revision, url.PathEscape(version)),
		CreationInfo: spdxCreationInfo{
			Created:  stamp.UTC().Format(time.RFC3339),
			Creators: []string{"Tool: github.com/d2lang/d2/ci/release/releasetool"},
		},
		Packages: []spdxPackage{{
			Name:             mainModule,
			SPDXID:           mainID,
			VersionInfo:      version,
			DownloadLocation: "git+https://github.com/d2lang/d2.git@" + revision,
			FilesAnalyzed:    false,
			LicenseConcluded: noAssertion,
			LicenseDeclared:  "MPL-2.0",
			CopyrightText:    noAssertion,
			Comment:          "Built with " + build.GoVersion,
			ExternalReferences: []spdxExternalRef{{
				Category: "PACKAGE-MANAGER",
				Type:     "purl",
				Locator:  goModulePURL(mainModule, version),
			}},
		}},
		Relationships: []spdxRelationship{{
			Element: "SPDXRef-DOCUMENT",
			Type:    "DESCRIBES",
			Related: mainID,
		}},
	}

	dependencies := append([]*debug.Module(nil), build.Deps...)
	sort.Slice(dependencies, func(i, j int) bool {
		return dependencies[i].Path < dependencies[j].Path
	})
	for index, dependency := range dependencies {
		modulePath := dependency.Path
		moduleVersion := dependency.Version
		comment := ""
		if dependency.Sum != "" {
			comment = "Go module checksum: " + dependency.Sum
		}
		if dependency.Replace != nil {
			replacement := dependency.Replace
			if comment != "" {
				comment += "; "
			}
			comment += "replaced by " + strings.TrimSpace(replacement.Path+" "+replacement.Version)
			modulePath = replacement.Path
			moduleVersion = replacement.Version
		}
		packageID := fmt.Sprintf("SPDXRef-Package-%d", index+1)
		document.Packages = append(document.Packages, spdxPackage{
			Name:             modulePath,
			SPDXID:           packageID,
			VersionInfo:      moduleVersion,
			DownloadLocation: noAssertion,
			FilesAnalyzed:    false,
			LicenseConcluded: noAssertion,
			LicenseDeclared:  noAssertion,
			CopyrightText:    noAssertion,
			Comment:          comment,
			ExternalReferences: []spdxExternalRef{{
				Category: "PACKAGE-MANAGER",
				Type:     "purl",
				Locator:  goModulePURL(modulePath, moduleVersion),
			}},
		})
		document.Relationships = append(document.Relationships, spdxRelationship{
			Element: mainID,
			Type:    "DEPENDS_ON",
			Related: packageID,
		})
	}
	return document, nil
}

func goModulePURL(modulePath, version string) string {
	escapedPath := strings.ReplaceAll(url.PathEscape(modulePath), "%2F", "/")
	purl := "pkg:golang/" + escapedPath
	if version != "" {
		purl += "@" + url.PathEscape(version)
	}
	return purl
}

func verifyChecksums(manifestPath, directory string, expectedCount int) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	if expectedCount >= 0 && len(lines) != expectedCount {
		return fmt.Errorf("checksum manifest has %d entries, expected %d", len(lines), expectedCount)
	}
	seen := map[string]bool{}
	for lineNumber, line := range lines {
		if len(line) < 67 || line[64:66] != "  " {
			return fmt.Errorf("invalid checksum manifest line %d", lineNumber+1)
		}
		digest := line[:64]
		if _, err := hex.DecodeString(digest); err != nil {
			return fmt.Errorf("invalid checksum on line %d: %w", lineNumber+1, err)
		}
		name := line[66:]
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			return fmt.Errorf("invalid checksum subject %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate checksum subject %q", name)
		}
		seen[name] = true
		actual, err := fileSHA256(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		if actual != digest {
			return fmt.Errorf("SHA-256 mismatch for %s: got %s, want %s", name, actual, digest)
		}
	}
	return nil
}

func fileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyBinary(binaryPath, goos, goarch, goVersion, revision, vcsModified string, maxSize int64) error {
	info, err := os.Stat(binaryPath)
	if err != nil {
		return err
	}
	if info.Size() <= 0 || info.Size() > maxSize {
		return fmt.Errorf("binary size %d is outside the allowed range 1..%d", info.Size(), maxSize)
	}
	build, err := buildinfo.ReadFile(binaryPath)
	if err != nil {
		return err
	}
	if build.GoVersion != goVersion {
		return fmt.Errorf("binary uses %s, expected %s", build.GoVersion, goVersion)
	}
	if build.Path != mainModule {
		return fmt.Errorf("binary main package is %q, expected %q", build.Path, mainModule)
	}
	settings := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
	}
	wantSettings := map[string]string{
		"-buildmode":   "exe",
		"-compiler":    "gc",
		"-trimpath":    "true",
		"CGO_ENABLED":  "0",
		"GOARCH":       goarch,
		"GOOS":         goos,
		"vcs":          "git",
		"vcs.modified": vcsModified,
		"vcs.revision": revision,
	}
	if goarch == "amd64" {
		wantSettings["GOAMD64"] = "v1"
	}
	if goarch == "arm64" {
		wantSettings["GOARM64"] = "v8.0"
	}
	for key, want := range wantSettings {
		if got := settings[key]; got != want {
			return fmt.Errorf("binary build setting %s is %q, expected %q", key, got, want)
		}
	}
	return verifyStripped(binaryPath, goos)
}

func verifyStripped(binaryPath, goos string) error {
	switch goos {
	case "linux":
		file, err := elf.Open(binaryPath)
		if err != nil {
			return err
		}
		defer file.Close()
		if file.Section(".symtab") != nil || file.Section(".debug_info") != nil {
			return errors.New("ELF binary still contains a symbol table or DWARF")
		}
		if file.Section(".gopclntab") == nil || file.Section(".go.buildinfo") == nil {
			return errors.New("ELF binary is missing Go stack or build metadata")
		}
	case "darwin":
		file, err := macho.Open(binaryPath)
		if err != nil {
			return err
		}
		defer file.Close()
		for _, section := range file.Sections {
			if section.Seg == "__DWARF" || strings.HasPrefix(section.Name, "__debug_") {
				return errors.New("Mach-O binary still contains DWARF")
			}
		}
		if file.Section("__gopclntab") == nil || file.Section("__go_buildinfo") == nil {
			return errors.New("Mach-O binary is missing Go stack or build metadata")
		}
		if file.Symtab != nil {
			for _, symbol := range file.Symtab.Syms {
				if symbol.Name == "main.main" || symbol.Name == "runtime.main" || symbol.Name == "_main.main" || symbol.Name == "_runtime.main" {
					return errors.New("Mach-O binary still contains Go linker symbols")
				}
			}
		}
	case "windows":
		file, err := pe.Open(binaryPath)
		if err != nil {
			return err
		}
		defer file.Close()
		for _, section := range file.Sections {
			if strings.HasPrefix(section.Name, ".debug_") {
				return errors.New("PE binary still contains DWARF")
			}
		}
		for _, symbol := range file.Symbols {
			if symbol.Name == "main.main" || symbol.Name == "runtime.main" {
				return errors.New("PE binary still contains Go linker symbols")
			}
		}
	default:
		return fmt.Errorf("unsupported release GOOS %q", goos)
	}
	return nil
}

func verifyArchive(archivePath, root string, stamp time.Time, maxSize int64) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return err
	}
	if info.Size() <= 0 || info.Size() > maxSize {
		return fmt.Errorf("archive size %d is outside the allowed range 1..%d", info.Size(), maxSize)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	zr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer zr.Close()
	if !zr.Header.ModTime.Equal(stamp) {
		return fmt.Errorf("gzip timestamp is %s, expected %s", zr.Header.ModTime, stamp)
	}

	required := map[string]bool{
		root + "/":                        false,
		root + "/LICENSE.txt":             false,
		root + "/THIRD_PARTY_NOTICES.txt": false,
		root + "/Makefile":                false,
		root + "/README.md":               false,
		root + "/man/d2.1":                false,
		root + "/scripts/install.sh":      false,
		root + "/scripts/lib.sh":          false,
		root + "/scripts/uninstall.sh":    false,
	}
	binaryPrefix := root + "/bin/d2"
	binaryFound := false
	previous := ""
	count := 0
	tr := tar.NewReader(zr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		count++
		name := header.Name
		clean := path.Clean(name)
		if strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("unsafe archive path %q", name)
		}
		if name != root+"/" && !strings.HasPrefix(name, root+"/") {
			return fmt.Errorf("archive entry %q is outside %q", name, root)
		}
		if previous != "" && name <= previous {
			return fmt.Errorf("archive entries are not strictly sorted: %q follows %q", name, previous)
		}
		previous = name
		if !header.ModTime.Equal(stamp) || !header.AccessTime.IsZero() || !header.ChangeTime.IsZero() {
			return fmt.Errorf("archive entry %q has non-normalized timestamps", name)
		}
		if header.Uid != 0 || header.Gid != 0 || header.Uname != "" || header.Gname != "" {
			return fmt.Errorf("archive entry %q has non-normalized ownership", name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if header.Mode != 0o755 || !strings.HasSuffix(name, "/") {
				return fmt.Errorf("archive directory %q has mode %#o or no trailing slash", name, header.Mode)
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Mode != 0o644 && header.Mode != 0o755 {
				return fmt.Errorf("archive file %q has mode %#o", name, header.Mode)
			}
		case tar.TypeSymlink:
			if header.Mode != 0o777 {
				return fmt.Errorf("archive symlink %q has mode %#o", name, header.Mode)
			}
			if err := validateSymlink(name, header.Linkname, root); err != nil {
				return err
			}
		default:
			return fmt.Errorf("archive entry %q has unsupported type %d", name, header.Typeflag)
		}
		if _, ok := required[name]; ok {
			required[name] = true
		}
		if name == binaryPrefix || name == binaryPrefix+".exe" {
			if header.Typeflag != tar.TypeReg || header.Mode != 0o755 || header.Size <= 0 {
				return fmt.Errorf("archive binary %q is invalid", name)
			}
			binaryFound = true
		}
	}
	if count == 0 {
		return errors.New("archive is empty")
	}
	if !binaryFound {
		return errors.New("archive does not contain a D2 binary")
	}
	for name, found := range required {
		if !found {
			return fmt.Errorf("archive is missing %s", name)
		}
	}
	return nil
}

func validateSymlink(name, target, root string) error {
	if target == "" || path.IsAbs(target) || strings.ContainsAny(target, "\\\x00") || (len(target) >= 2 && target[1] == ':') {
		return fmt.Errorf("archive symlink %q has unsafe target %q", name, target)
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	if resolved != root && !strings.HasPrefix(resolved, root+"/") {
		return fmt.Errorf("archive symlink %q escapes %q with target %q", name, root, target)
	}
	return nil
}

func writeFileAtomic(output string, data []byte, mode fs.FileMode) (retErr error) {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(output), "."+filepath.Base(output)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if retErr != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Remove(output); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpName, output)
}
