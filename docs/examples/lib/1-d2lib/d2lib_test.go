package main

import (
	"context"
	"testing"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/go2"
)

func TestMain_(t *testing.T) {
	main()
}

func TestConfigHash(t *testing.T) {
	var hash1, hash2 string
	var err error

	{
		ruler, _ := textmeasure.NewRuler()
		layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
			return d2dagrelayout.DefaultLayout, nil
		}
		renderOpts := &d2svg.RenderOpts{
			Pad:     go2.Pointer(int64(5)),
			ThemeID: &d2themescatalog.GrapeSoda.ID,
		}
		compileOpts := &d2lib.CompileOptions{
			LayoutResolver: layoutResolver,
			Ruler:          ruler,
		}
		ctx := log.WithDefault(context.Background())
		diagram, _, _ := d2lib.Compile(ctx, "x -> y", compileOpts, renderOpts)
		hash1, err = diagram.HashID(nil)
		assert.Success(t, err)
	}

	{
		ruler, _ := textmeasure.NewRuler()
		layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
			return d2dagrelayout.DefaultLayout, nil
		}
		renderOpts := &d2svg.RenderOpts{
			Pad:     go2.Pointer(int64(5)),
			ThemeID: &d2themescatalog.NeutralGrey.ID,
		}
		compileOpts := &d2lib.CompileOptions{
			LayoutResolver: layoutResolver,
			Ruler:          ruler,
		}
		ctx := log.WithDefault(context.Background())
		diagram, _, _ := d2lib.Compile(ctx, "x -> y", compileOpts, renderOpts)
		hash2, err = diagram.HashID(nil)
		assert.Success(t, err)
	}

	assert.NotEqual(t, hash1, hash2)
}

func TestHashSalt(t *testing.T) {
	var hash1, hash2 string
	var err error

	{
		ruler, _ := textmeasure.NewRuler()
		layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
			return d2dagrelayout.DefaultLayout, nil
		}
		renderOpts := &d2svg.RenderOpts{
			Pad:     go2.Pointer(int64(5)),
			ThemeID: &d2themescatalog.GrapeSoda.ID,
		}
		compileOpts := &d2lib.CompileOptions{
			LayoutResolver: layoutResolver,
			Ruler:          ruler,
		}
		ctx := log.WithDefault(context.Background())
		diagram, _, _ := d2lib.Compile(ctx, "x -> y", compileOpts, renderOpts)
		hash1, err = diagram.HashID(nil)
		assert.Success(t, err)
	}

	{
		ruler, _ := textmeasure.NewRuler()
		layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
			return d2dagrelayout.DefaultLayout, nil
		}
		renderOpts := &d2svg.RenderOpts{
			Pad:     go2.Pointer(int64(5)),
			ThemeID: &d2themescatalog.GrapeSoda.ID,
		}
		compileOpts := &d2lib.CompileOptions{
			LayoutResolver: layoutResolver,
			Ruler:          ruler,
		}
		ctx := log.WithDefault(context.Background())
		diagram, _, _ := d2lib.Compile(ctx, "x -> y", compileOpts, renderOpts)
		hash2, err = diagram.HashID(go2.Pointer("asdf"))
		assert.Success(t, err)
	}

	assert.NotEqual(t, hash1, hash2)
}
