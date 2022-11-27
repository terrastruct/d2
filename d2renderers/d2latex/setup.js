const adaptor = MathJax._.adaptors.liteAdaptor.liteAdaptor();
MathJax._.handlers.html_ts.RegisterHTMLHandler(adaptor)
const html = MathJax._.mathjax.mathjax.document('', {
  InputJax: new MathJax._.input.tex_ts.TeX(),
  OutputJax: new MathJax._.output.svg_ts.SVG({ fontCache: "none" }),
});
