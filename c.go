//go:build cgo

package d2

import "oss.terrastruct.com/d2/d2layouts/d2dagrelayout"

func init() {
	dagreLayout = d2dagrelayout.Layout
}
