package d2latex

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"testing"
)

func TestRender(t *testing.T) {
	txts := []string{
		`a + b = c`,
		`\frac{1}{2}`,
		`a + b
= c
`,
	}
	for _, txt := range txts {
		svg, err := Render(txt)
		if err != nil {
			t.Fatal(err)
		}
		var xmlParsed interface{}
		if err := xml.Unmarshal([]byte(svg), &xmlParsed); err != nil {
			t.Fatalf("invalid SVG: %v", err)
		}
	}
}

func TestD2SourceBackslashParity(t *testing.T) {
	const (
		tex      = `\bra{a}\ket{b}`
		wantHash = "c58251f49b0317576946c88910847352c05e809e534f299cc4f3c12ce7e66bef"
		wantW    = 37
		wantH    = 19
	)

	svg, err := Render(tex)
	if err != nil {
		t.Fatal(err)
	}
	gotHash := fmt.Sprintf("%x", sha256.Sum256([]byte(svg)))
	if gotHash != wantHash {
		t.Fatalf("SVG SHA-256 = %s, want %s", gotHash, wantHash)
	}
	width, height, err := Measure(tex)
	if err != nil {
		t.Fatal(err)
	}
	if width != wantW || height != wantH {
		t.Fatalf("Measure() = (%d, %d), want (%d, %d)", width, height, wantW, wantH)
	}
}

func TestFrozenMathJaxParity(t *testing.T) {
	tests := []struct {
		name          string
		tex           string
		sha256        string
		width, height int
	}{
		{"basic", `a + b = c`, "39400be50e4d495cfb3c1704b68c38ab95597ea69bfbdbecef9c502343636f63", 72, 15},
		{"fraction", `\frac{1}{2}`, "f47560ab694c6a16451e73ce35b8480492153f2d3bb0f8323f7f295e3a42c68d", 18, 37},
		{"multiline", "a + b\n= c\n", "39400be50e4d495cfb3c1704b68c38ab95597ea69bfbdbecef9c502343636f63", 72, 15},
		{"scripts", `x_i^2+\sum_{n=0}^{\infty}n^{-2}`, "936458024f85e6628d73f2712b72b8f822aecd0d90d521740d0c2e27ae087280", 99, 51},
		{"delimiters", `\left\langle \frac{x}{y}\middle|z\right\rangle`, "0f3bd3fd536b90f6967d072311523481aa1e2d248afe7130cb446994d385296a", 60, 44},
		{"ams_matrix", `\begin{pmatrix}a&b\\c&d\end{pmatrix}`, "048e79bc11a8a6c287df5738697b99e7c52e05b01c15edc51289c5a04591efae", 64, 44},
		{"ams_align", `\begin{aligned}a&=b+c\\d&=e\end{aligned}`, "beea3e49eedda60558d71b4588c4c512c549123baa428013cefe1effbbd3e514", 72, 42},
		{"mathtools", `\min_{\mathclap{\substack{x\in\mathbb{R}^n\\x\geq0\\Ax\leq b}}}c^Tx`, "68dc56f03c3e1d51fcd77c7cb565cf17ae91e27f86e4bfa112bc3a2dd11105ea", 62, 55},
		{"amscd", `\begin{CD}A@>f>>B\\@VgVV@VVhV\\C@>>k>D\end{CD}`, "11db6e93c6718a81f80c269a01600fb0ddee08f30bd5765d323396400cc1de8b", 106, 112},
		{"braket", `\bra{\psi}A\ket{\phi}+\braket{x|y}`, "03df435de3e87e27eef1a9a10db840269ceebe1c8190dcac2b657a556d464f7b", 154, 19},
		{"cancel", `\cancel{x}+\bcancel{y}+\xcancel{z}+\cancelto{0}{w}`, "0c5aeba8d46e5a5f278a5a537d6687132f651f75b4d6717e5c6dee18b38d2a81", 155, 26},
		{"cases", `f(x)=\begin{cases}x^2&x\geq0\\-x&x<0\end{cases}`, "dd018141ff8cb02c1a2e5994998725a9966bdbf3c4bab05b48ae3df8e0f31d85", 159, 44},
		{"color", `\color{red}{x}+\colorbox{yellow}{y}`, "3cf885ed975f83b61c3977859092ebfdf1f18f85ecac23d7dfd866f19e5b1090", 53, 22},
		{"gensymb", `90\degree+20\celsius+5\micro\ohm`, "cc48530596511f31c9e3ac34819cad8cb1b229089663e94941b830f951b7fcfe", 134, 18},
		{"mhchem", `\ce{2H2 + O2 -> 2H2O}`, "32a36be4fc7ebaa02b6369c3acd571c11f16a8a9836b5a01d2c4437c70f67016", 165, 16},
		{"physics", `\dv{x}f(x)+\qty(\frac{a}{b})`, "b342affabb06996648d52fffcabfbf2f82b19b1abdd82fadd9ec8138430d5df8", 125, 38},
		{"invalid_merror", `\frac{1}{`, "c2d6ee6d434a5313546972d803487c1f505ea5ad70c94f94997cec11faef148b", 207, 18},
		{"d2_huge", `\Huge{\frac{\alpha g^2}{\omega^5} e^{[ -0.74\bigl\{\frac{\omega U_\omega 19.5}{g}\bigr\}^{\!-4}\,]}}`, "a200593e75830539243054b9da91acbe21bedd792c7147644cbb6bd14a7255b1", 382, 101},
		{"d2_emc2", `e = mc^2`, "e724fceadd904173810ae62ff609e48a78f1a777105a10cc23a21fce45f77052", 65, 18},
		{"d2_gibberish_sum", `gibberish\; math:\sum_{i=0}^\infty i^2`, "e0e37a92d36c8a44a9ab45c5fa97d5c7bcafade0e9c79169bcce9d6832ea3dcf", 179, 51},
		{"d2_linear_program", `\min_{ \mathclap{\substack{ x \in \mathbb{R}^n \ x \geq 0 \ Ax \leq b }}} c^T x`, "ff6a130d0d20e442931ba22d8f576feae1c47fcc7118e11957d34645232cedb1", 62, 32},
		{"d2_equation_split", "\\begin{equation} \\label{eq1}\n\\begin{split}\nA & = \\frac{\\\\pi r^2}{2} \\\\\n & = \\frac{1}{2} \\pi r^2\n\\end{split}\n\\end{equation}", "e00876f4f1c4aae7d7da24065e54f71cd940ed71d3f4edadb34fae406fc5a443", 82, 82},
		{"d2_limit", `\lim_{h \rightarrow 0 } \frac{f(x+h)-f(x)}{h}`, "4c33befe8c902c4ea5a2e30c62c978d9bacfc16d3c067cccc743c457446bc0b3", 162, 41},
		{"d2_quadratic", `f(x) = x^2 + 2x + 1`, "57c71028f1aa785897e79afe61723d9beb11ce56372247cba6480f2380083ae2", 150, 21},
		{"d2_physics_plugin", "\\var{F[g(x)]}\n\\dd(\\cos\\theta)", "4e6c3221a2cb2c21d32c8d66a51294f0839f03121a32e43e38e7d0ed89d828a7", 128, 19},
		{"d2_displaylines", "\\displaylines{x = a + b \\\\ y = b + c}\n\\sum_{k=1}^{n} h_{k} \\int_{0}^{1} \\bigl(\\partial_{k} f(x_{k-1}+t h_{k} e_{k}) -\\partial_{k} f(a)\\bigr) \\,dt", "8fa7020a6fa463de3a6bd656498cf9fd62518bda834f0895a6f42b7dbb8abbcf", 404, 52},
		{"d2_add", `1 + 1`, "b6eacdbf2d6531e407bfadc5d81bd3d5dc472bffa23cbc1908437553b0cfa072", 41, 14},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svg, err := Render(test.tex)
			if err != nil {
				t.Fatal(err)
			}
			gotHash := fmt.Sprintf("%x", sha256.Sum256([]byte(svg)))
			if gotHash != test.sha256 {
				t.Fatalf("SVG SHA-256 = %s, want %s", gotHash, test.sha256)
			}
			width, height, err := Measure(test.tex)
			if err != nil {
				t.Fatal(err)
			}
			if width != test.width || height != test.height {
				t.Fatalf("Measure() = (%d, %d), want (%d, %d)", width, height, test.width, test.height)
			}
		})
	}
}
