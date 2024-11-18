package textmeasure

import (
	"fmt"
	"regexp"
	"strings"
)

func sanitizeLinks(input string) (string, error) {
	re := regexp.MustCompile(`href="([^"]*)"`)

	return re.ReplaceAllStringFunc(input, func(href string) string {
		matches := re.FindStringSubmatch(href)
		if len(matches) < 2 {
			return href
		}

		value := matches[1]

		value = strings.ReplaceAll(value, "&amp;", "TEMP_AMP")
		value = strings.ReplaceAll(value, "&", "&amp;")
		value = strings.ReplaceAll(value, "TEMP_AMP", "&amp;")

		return fmt.Sprintf(`href="%s"`, value)
	}), nil
}
