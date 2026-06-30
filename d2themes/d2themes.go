package d2themes

import "oss.terrastruct.com/d2/lib/themes"

func GetTheme(id int) (*themes.Theme, error) {
	theme, err := themes.LoadTheme(id)
	if err!= nil {
		return nil, fmt.Errorf("failed to load theme %d: %v", id, err)
	}
	return theme, nil
}
