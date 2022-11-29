const adaptor = MathJax._.adaptors.liteAdaptor.liteAdaptor();
MathJax._.handlers.html_ts.RegisterHTMLHandler(adaptor)
const html = MathJax._.mathjax.mathjax.document('', {
  InputJax: new MathJax._.input.tex_ts.TeX({ packages: ['base', 'mathtools', 'amscd', 'braket', 'cancel', 'cases', 'color', 'gensymb', 'mhchem', 'physics'] }),
  OutputJax: new MathJax._.output.svg_ts.SVG(),
});
