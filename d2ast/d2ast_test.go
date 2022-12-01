package d2ast_test

import (
	"encoding/json"
	"math/big"
	math_rand "math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/xrand"

	"oss.terrastruct.com/util-go/diff"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
)

func TestRange(t *testing.T) {
	t.Parallel()

	t.Run("String", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			r    d2ast.Range
			exp  string
		}{
			{
				name: "one_byte",
				r: d2ast.Range{
					Path: "/src/example.go",
					Start: d2ast.Position{
						Line:   10,
						Column: 5,
						Byte:   100,
					},
					End: d2ast.Position{
						Line:   10,
						Column: 6,
						Byte:   100,
					},
				},
				exp: "/src/example.go:11:6",
			},
			{
				name: "more_than_one_byte",
				r: d2ast.Range{
					Path: "/src/example.go",
					Start: d2ast.Position{
						Line:   10,
						Column: 5,
						Byte:   100,
					},
					End: d2ast.Position{
						Line:   10,
						Column: 7,
						Byte:   101,
					},
				},
				exp: "/src/example.go:11:6",
			},
			{
				name: "empty_path",
				r: d2ast.Range{
					Start: d2ast.Position{
						Line:   10,
						Column: 5,
						Byte:   100,
					},
					End: d2ast.Position{
						Line:   10,
						Column: 7,
						Byte:   101,
					},
				},
				exp: "11:6",
			},
			{
				name: "start_equal_end",
				r: d2ast.Range{
					Start: d2ast.Position{
						Line:   10,
						Column: 5,
						Byte:   100,
					},
					End: d2ast.Position{
						Line:   10,
						Column: 5,
						Byte:   100,
					},
				},
				exp: "11:6",
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				if tc.exp != tc.r.String() {
					t.Fatalf("expected %q but got %q", tc.exp, tc.r.String())
				}
			})
		}
	})

	t.Run("UnmarshalText", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			in   string

			exp d2ast.Range

			errmsg string
		}{
			{
				name: "success",
				in:   `"json_test.d2,1:1:0-5:1:50"`,
				exp:  d2ast.Range{Path: "json_test.d2", Start: d2ast.Position{Line: 1, Column: 1, Byte: 0}, End: d2ast.Position{Line: 5, Column: 1, Byte: 50}},
			},
			{
				name:   "err1",
				in:     `"json_test.d2-5:1:50"`,
				errmsg: "missing Start field",
			},
			{
				name:   "err2",
				in:     `"json_test.d2"`,
				errmsg: "missing End field",
			},
			{
				name:   "err3",
				in:     `"json_test.d2,1:1:0-5:150"`,
				errmsg: "expected three fields",
			},
			{
				name:   "err4",
				in:     `"json_test.d2,1:10-5:1:50"`,
				errmsg: "expected three fields",
			},
			{
				name:   "err5",
				in:     `"json_test.d2,a:1:0-5:1:50"`,
				errmsg: `strconv.Atoi: parsing "a": invalid syntax`,
			},
			{
				name:   "err6",
				in:     `"json_test.d2,1:c:0-5:1:50"`,
				errmsg: `strconv.Atoi: parsing "c": invalid syntax`,
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var r d2ast.Range
				err := json.Unmarshal([]byte(tc.in), &r)

				if tc.errmsg != "" {
					if err == nil {
						t.Fatalf("expected error: %#v", err)
					}
					if !strings.Contains(err.Error(), tc.errmsg) {
						t.Fatalf("error message does not contain %q: %q", tc.errmsg, err.Error())
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(tc.exp, r) {
						t.Fatalf("expected %#v but got %#v", tc.exp, r)
					}
				}
			})
		}
	})

	t.Run("Advance", func(t *testing.T) {
		t.Parallel()

		t.Run("UTF-8", func(t *testing.T) {
			t.Parallel()

			var p d2ast.Position
			p = p.Advance('a', false)
			assert.StringJSON(t, `"0:1:1"`, p)
			p = p.Advance('\n', false)
			assert.StringJSON(t, `"1:0:2"`, p)
			p = p.Advance('è', false)
			assert.StringJSON(t, `"1:2:4"`, p)
			p = p.Advance('𐀀', false)
			assert.StringJSON(t, `"1:6:8"`, p)

			p = p.Subtract('𐀀', false)
			assert.StringJSON(t, `"1:2:4"`, p)
			p = p.Subtract('è', false)
			assert.StringJSON(t, `"1:0:2"`, p)
		})

		t.Run("UTF-16", func(t *testing.T) {
			t.Parallel()

			var p d2ast.Position
			p = p.Advance('a', true)
			assert.StringJSON(t, `"0:1:1"`, p)
			p = p.Advance('\n', true)
			assert.StringJSON(t, `"1:0:2"`, p)
			p = p.Advance('è', true)
			assert.StringJSON(t, `"1:1:3"`, p)
			p = p.Advance('𐀀', true)
			assert.StringJSON(t, `"1:3:5"`, p)

			p = p.Subtract('𐀀', true)
			assert.StringJSON(t, `"1:1:3"`, p)
			p = p.Subtract('è', true)
			assert.StringJSON(t, `"1:0:2"`, p)
		})
	})
}

func TestJSON(t *testing.T) {
	t.Parallel()

	m := &d2ast.Map{
		Range: d2ast.Range{Path: "json_test.d2", Start: d2ast.Position{Line: 0, Column: 0, Byte: 0}, End: d2ast.Position{Line: 5, Column: 1, Byte: 50}},

		Nodes: []d2ast.MapNodeBox{
			{
				Comment: &d2ast.Comment{
					Value: `America was discovered by Amerigo Vespucci and was named after him, until
people got tired of living in a place called "Vespuccia" and changed its
name to "America".
		-- Mike Harding, "The Armchair Anarchist's Almanac"`,
				},
			},
			{
				BlockComment: &d2ast.BlockComment{
					Value: `America was discovered by Amerigo Vespucci and was named after him, until
people got tired of living in a place called "Vespuccia" and changed its
name to "America".
		-- Mike Harding, "The Armchair Anarchist's Almanac"`,
				},
			},
			{
				Substitution: &d2ast.Substitution{
					Spread: true,
					Path: []*d2ast.StringBox{
						{
							BlockString: &d2ast.BlockString{
								Quote: "|",
								Tag:   "text",
								Value: `America was discovered by Amerigo Vespucci and was named after him, until
	people got tired of living in a place called "Vespuccia" and changed its
	name to "America".
	-- Mike Harding, "The Armchair Anarchist's Almanac"`,
							},
						},
					},
				},
			},
			{
				MapKey: &d2ast.Key{
					Ampersand: true,

					Key: &d2ast.KeyPath{
						Path: []*d2ast.StringBox{
							{
								SingleQuotedString: &d2ast.SingleQuotedString{
									Value: "before edges",
								},
							},
						},
					},

					Edges: []*d2ast.Edge{
						{
							Src: &d2ast.KeyPath{
								Path: []*d2ast.StringBox{
									{
										SingleQuotedString: &d2ast.SingleQuotedString{
											Value: "src",
										},
									},
								},
							},
							SrcArrow: "*",

							Dst: &d2ast.KeyPath{
								Path: []*d2ast.StringBox{
									{
										SingleQuotedString: &d2ast.SingleQuotedString{
											Value: "dst",
										},
									},
								},
							},
							DstArrow: ">",
						},
						{
							Src: &d2ast.KeyPath{
								Path: []*d2ast.StringBox{
									{
										SingleQuotedString: &d2ast.SingleQuotedString{
											Value: "dst",
										},
									},
								},
							},

							Dst: &d2ast.KeyPath{
								Path: []*d2ast.StringBox{
									{
										SingleQuotedString: &d2ast.SingleQuotedString{
											Value: "dst2",
										},
									},
								},
							},
						},
					},

					EdgeIndex: &d2ast.EdgeIndex{
						Glob: true,
					},

					EdgeKey: &d2ast.KeyPath{
						Path: []*d2ast.StringBox{
							{
								SingleQuotedString: &d2ast.SingleQuotedString{
									Value: "after edges",
								},
							},
						},
					},

					Primary: d2ast.ScalarBox{
						Null: &d2ast.Null{},
					},

					Value: d2ast.ValueBox{
						Array: &d2ast.Array{
							Nodes: []d2ast.ArrayNodeBox{
								{
									Boolean: &d2ast.Boolean{
										Value: true,
									},
								},
								{
									Number: &d2ast.Number{
										Raw:   "0xFF",
										Value: big.NewRat(15, 1),
									},
								},
								{
									UnquotedString: &d2ast.UnquotedString{
										Value: []d2ast.InterpolationBox{
											{
												String: go2.Pointer("no quotes needed"),
											},
										},
									},
								},
								{
									UnquotedString: &d2ast.UnquotedString{
										Value: []d2ast.InterpolationBox{
											{
												Substitution: &d2ast.Substitution{},
											},
										},
									},
								},
								{
									DoubleQuotedString: &d2ast.DoubleQuotedString{
										Value: []d2ast.InterpolationBox{
											{
												String: go2.Pointer("no quotes needed"),
											},
										},
									},
								},
								{
									SingleQuotedString: &d2ast.SingleQuotedString{
										Value: "rawr",
									},
								},
								{
									BlockString: &d2ast.BlockString{
										Quote: "|",
										Tag:   "text",
										Value: `America was discovered by Amerigo Vespucci and was named after him, until
			people got tired of living in a place called "Vespuccia" and changed its
			name to "America".
			-- Mike Harding, "The Armchair Anarchist's Almanac"`,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	assert.StringJSON(t, `{
  "range": "json_test.d2,0:0:0-5:1:50",
  "nodes": [
    {
      "comment": {
        "range": ",0:0:0-0:0:0",
        "value": "America was discovered by Amerigo Vespucci and was named after him, until\npeople got tired of living in a place called \"Vespuccia\" and changed its\nname to \"America\".\n\t\t-- Mike Harding, \"The Armchair Anarchist's Almanac\""
      }
    },
    {
      "block_comment": {
        "range": ",0:0:0-0:0:0",
        "value": "America was discovered by Amerigo Vespucci and was named after him, until\npeople got tired of living in a place called \"Vespuccia\" and changed its\nname to \"America\".\n\t\t-- Mike Harding, \"The Armchair Anarchist's Almanac\""
      }
    },
    {
      "substitution": {
        "range": ",0:0:0-0:0:0",
        "spread": true,
        "path": [
          {
            "block_string": {
              "range": ",0:0:0-0:0:0",
              "quote": "|",
              "tag": "text",
              "value": "America was discovered by Amerigo Vespucci and was named after him, until\n\tpeople got tired of living in a place called \"Vespuccia\" and changed its\n\tname to \"America\".\n\t-- Mike Harding, \"The Armchair Anarchist's Almanac\""
            }
          }
        ]
      }
    },
    {
      "map_key": {
        "range": ",0:0:0-0:0:0",
        "ampersand": true,
        "key": {
          "range": ",0:0:0-0:0:0",
          "path": [
            {
              "single_quoted_string": {
                "range": ",0:0:0-0:0:0",
                "raw": "",
                "value": "before edges"
              }
            }
          ]
        },
        "edges": [
          {
            "range": ",0:0:0-0:0:0",
            "src": {
              "range": ",0:0:0-0:0:0",
              "path": [
                {
                  "single_quoted_string": {
                    "range": ",0:0:0-0:0:0",
                    "raw": "",
                    "value": "src"
                  }
                }
              ]
            },
            "src_arrow": "*",
            "dst": {
              "range": ",0:0:0-0:0:0",
              "path": [
                {
                  "single_quoted_string": {
                    "range": ",0:0:0-0:0:0",
                    "raw": "",
                    "value": "dst"
                  }
                }
              ]
            },
            "dst_arrow": ">"
          },
          {
            "range": ",0:0:0-0:0:0",
            "src": {
              "range": ",0:0:0-0:0:0",
              "path": [
                {
                  "single_quoted_string": {
                    "range": ",0:0:0-0:0:0",
                    "raw": "",
                    "value": "dst"
                  }
                }
              ]
            },
            "src_arrow": "",
            "dst": {
              "range": ",0:0:0-0:0:0",
              "path": [
                {
                  "single_quoted_string": {
                    "range": ",0:0:0-0:0:0",
                    "raw": "",
                    "value": "dst2"
                  }
                }
              ]
            },
            "dst_arrow": ""
          }
        ],
        "edge_index": {
          "range": ",0:0:0-0:0:0",
          "int": null,
          "glob": true
        },
        "edge_key": {
          "range": ",0:0:0-0:0:0",
          "path": [
            {
              "single_quoted_string": {
                "range": ",0:0:0-0:0:0",
                "raw": "",
                "value": "after edges"
              }
            }
          ]
        },
        "primary": {
          "null": {
            "range": ",0:0:0-0:0:0"
          }
        },
        "value": {
          "array": {
            "range": ",0:0:0-0:0:0",
            "nodes": [
              {
                "boolean": {
                  "range": ",0:0:0-0:0:0",
                  "value": true
                }
              },
              {
                "number": {
                  "range": ",0:0:0-0:0:0",
                  "raw": "0xFF",
                  "value": "15"
                }
              },
              {
                "unquoted_string": {
                  "range": ",0:0:0-0:0:0",
                  "value": [
                    {
                      "string": "no quotes needed"
                    }
                  ]
                }
              },
              {
                "unquoted_string": {
                  "range": ",0:0:0-0:0:0",
                  "value": [
                    {
                      "substitution": {
                        "range": ",0:0:0-0:0:0",
                        "spread": false,
                        "path": null
                      }
                    }
                  ]
                }
              },
              {
                "double_quoted_string": {
                  "range": ",0:0:0-0:0:0",
                  "value": [
                    {
                      "string": "no quotes needed"
                    }
                  ]
                }
              },
              {
                "single_quoted_string": {
                  "range": ",0:0:0-0:0:0",
                  "raw": "",
                  "value": "rawr"
                }
              },
              {
                "block_string": {
                  "range": ",0:0:0-0:0:0",
                  "quote": "|",
                  "tag": "text",
                  "value": "America was discovered by Amerigo Vespucci and was named after him, until\n\t\t\tpeople got tired of living in a place called \"Vespuccia\" and changed its\n\t\t\tname to \"America\".\n\t\t\t-- Mike Harding, \"The Armchair Anarchist's Almanac\""
                }
              }
            ]
          }
        }
      }
    }
  ]
}`, m)
}

func testRawStringKey(t *testing.T, key string) {
	ast := d2ast.RawString(key, true)
	enc := d2format.Format(ast)
	k, err := d2parser.ParseKey(enc)
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Path) != 1 {
		t.Fatalf("unexpected key length: %#v", k.Path)
	}
	err = diff.Runes(key, k.Path[0].Unbox().ScalarString())
	if err != nil {
		t.Fatal(err)
	}
}

func testRawStringValue(t *testing.T, value string) {
	ast := d2ast.RawString(value, false)
	enc := d2format.Format(ast)
	v, err := d2parser.ParseValue(enc)
	if err != nil {
		t.Fatal(err)
	}
	ps, ok := v.(d2ast.Scalar)
	if !ok {
		t.Fatalf("unexpected value type: %#v", v)
	}
	err = diff.Runes(value, ps.ScalarString())
	if err != nil {
		t.Fatal(err)
	}
}

func TestRawString(t *testing.T) {
	t.Parallel()

	t.Run("chaos", func(t *testing.T) {
		t.Parallel()

		t.Run("pinned", func(t *testing.T) {
			t.Parallel()

			pinnedTestCases := []struct {
				name string
				str  string
			}{
				{
					name: "1",
					str:  "\U000b64cd\U0008b732\U0009632c\U000983f8\U000f42d4\U000c4749\U00041723\uf584蝉\U00100cd5\U0003325d\U0003e4d2\U0007ff0e\U000e03d8\U000b0431\U00042053\U0001b3ea𠒹\U0006d9cf\U000c5b1c\U00019a3c\U000f3c3d\U0004acedଶ\U0009da18\U0001a0bb\U000b6bfd\U00015ebd\U00088c5a녈\U00078277\U000eaa58\U0009266b\U000d85ae\U000d6ce8譊𣱡\U0008ac84\U000a722f\U000d3d35\U00072581\U000c3423\U000a1753\U00082014\U0001bde6\U0010bf47炏\U000423fa\U0007df70\U00088aaf\U00074e5e\U000ee80b\U000e3d53\U0003f542\U0001ad9f\U00031408\U000cce7e\U00082172\u202f",
				},
				{
					name: "2",
					str:  "'\"Tc\U000d148d\U000dd61a\U0007cf68OO\U000b87a9\U000c073a\U000e7828n\U00068a9fc\U0004fbf5\x041\\'''",
				},
				{
					name: "3",
					str:  "\r\U00057d53\x01'\U00042e5a\U0007be73T\U000fb916\x01\U000e0e4afL]\U000474d1\x15\U00083bc0\fbT\ue09bs{vP\U000b3d33\x0f\U0007ad13\x10\U00098b38\x1d\U000cf9da\n ",
				},
			}
			for _, tc := range pinnedTestCases {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					t.Run("key", func(t *testing.T) {
						t.Parallel()
						testRawStringKey(t, tc.str)
					})

					t.Run("value", func(t *testing.T) {
						t.Parallel()
						testRawStringValue(t, tc.str)
					})
				})
			}
		})

		for i := 0; i < 1000; i++ {
			i := i
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()

				s := xrand.String(math_rand.Intn(99), nil)
				t.Logf("testing: %q", s)

				t.Run("key", func(t *testing.T) {
					t.Parallel()
					testRawStringKey(t, s)
				})

				t.Run("value", func(t *testing.T) {
					t.Parallel()
					testRawStringValue(t, s)
				})
			})
		}
	})

	testCases := []struct {
		name  string
		str   string
		exp   string
		inKey bool
	}{
		{
			name: "empty",
			str:  ``,
			exp:  `""`,
		},
		{
			name: "null",
			str:  `null`,
			exp:  `"null"`,
		},
		{
			name: "simple",
			str:  `wearisome_condition_of_humanity`,
			exp:  `wearisome_condition_of_humanity`,
		},
		{
			name: "specials_double",
			str:  `'#;#;#'`,
			exp:  `"'#;#;#'"`,
		},
		{
			name: "specials_single_quote",
			str:  `"cambridge"`,
			exp:  `'"cambridge"'`,
		},
		{
			name: "specials_single_dollar",
			str:  `$bingo`,
			exp:  `'$bingo'`,
		},
		{
			name: "not_key_specials",
			str:  `------`,
			exp:  `------`,
		},
		{
			name:  "key_specials_double",
			str:   `-----`,
			exp:   `"-----"`,
			inKey: true,
		},
		{
			name:  "key_specials_single",
			str:   `"cambridge"`,
			exp:   `'"cambridge"'`,
			inKey: true,
		},
		{
			name:  "key_specials_unquoted",
			str:   `square-2`,
			exp:   `square-2`,
			inKey: true,
		},
		{
			name: "multiline",
			str: `||||yes
yes
yes
yes
||||`,
			exp:   `"||||yes\nyes\nyes\nyes\n||||"`,
			inKey: true,
		},
		{
			name: "leading_whitespace",
			str:  `  yoho_park   `,
			exp:  `"  yoho_park   "`,
		},
		{
			name: "leading_whitespace_newlines",
			str: `  yoho
_park   `,
			exp: `"  yoho\n_park   "`,
		},
		{
			name: "leading_space_double_quotes_and_newlines",
			str: `   "yoho"
_park   `,
			exp: `"   \"yoho\"\n_park   "`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ast := d2ast.RawString(tc.str, tc.inKey)
			assert.String(t, tc.exp, d2format.Format(ast))
		})
	}
}
