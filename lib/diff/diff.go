package diff

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"oss.terrastruct.com/diff"
)

// TODO refactor with diff repo
func TestdataGeneric(path, fileExtension string, got []byte) (err error) {
	expPath := fmt.Sprintf("%s.exp%s", path, fileExtension)
	gotPath := fmt.Sprintf("%s.got%s", path, fileExtension)

	err = os.MkdirAll(filepath.Dir(gotPath), 0755)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(gotPath, got, 0600)
	if err != nil {
		return err
	}

	ds, err := diff.Files(expPath, gotPath)
	if err != nil {
		return err
	}

	if ds != "" {
		if os.Getenv("TESTDATA_ACCEPT") != "" {
			return os.Rename(gotPath, expPath)
		}
		return fmt.Errorf("diff (rerun with $TESTDATA_ACCEPT=1 to accept):\n%s", ds)
	}
	return os.Remove(gotPath)
}
