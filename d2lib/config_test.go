package d2lib

import (
	"testing"

	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
)

func TestApplyConfigsThemeOverridePrecedence(t *testing.T) {
	t.Parallel()

	configLight := "#abcdef"
	configDark := "#fedcba"
	callerLight := "#112233"
	callerDark := "#332211"
	config := &d2target.Config{
		ThemeOverrides:     &d2target.ThemeOverrides{B1: &configLight},
		DarkThemeOverrides: &d2target.ThemeOverrides{B1: &configDark},
	}

	t.Run("caller options win", func(t *testing.T) {
		renderOpts := &d2svg.RenderOpts{
			ThemeOverrides:     &d2target.ThemeOverrides{B1: &callerLight},
			DarkThemeOverrides: &d2target.ThemeOverrides{B1: &callerDark},
		}
		applyConfigs(config, &CompileOptions{}, renderOpts)

		if got := *renderOpts.ThemeOverrides.B1; got != callerLight {
			t.Fatalf("light theme override = %q, want caller value %q", got, callerLight)
		}
		if got := *renderOpts.DarkThemeOverrides.B1; got != callerDark {
			t.Fatalf("dark theme override = %q, want caller value %q", got, callerDark)
		}
	})

	t.Run("config fills absent options", func(t *testing.T) {
		renderOpts := &d2svg.RenderOpts{}
		applyConfigs(config, &CompileOptions{}, renderOpts)

		if got := *renderOpts.ThemeOverrides.B1; got != configLight {
			t.Fatalf("light theme override = %q, want config value %q", got, configLight)
		}
		if got := *renderOpts.DarkThemeOverrides.B1; got != configDark {
			t.Fatalf("dark theme override = %q, want config value %q", got, configDark)
		}
	})
}
