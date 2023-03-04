Diagrams that link between objects and the source they represent are much more integrated into your overall documentation than standalone diagrams. This release brings the linking feature to PDFs! Try clicking on "GitHub" object in the following PDF: 

[linked.pdf](https://github.com/terrastruct/d2/files/10889489/scratch.pdf)

Code blocks now adapt to dark mode:

<img width="643" alt="Screen Shot 2023-03-04 at 11 33 46 AM" src="https://user-images.githubusercontent.com/3120367/222925564-a5068bfa-e2d8-4358-b95a-cf48c41314f3.png">

Welcome new contributor @donglixiaoche , who helps D2 support border-radius on connections!
<img width="643" alt="Screen Shot 2023-03-04 at 11 33 46 AM" src="https://user-images.githubusercontent.com/3120367/222925369-ded99063-55c8-4330-92e7-0fd3f22a03eb.png">


#### Features 🚀

- PDF exports support linking [#891](https://github.com/terrastruct/d2/issues/891), [#966](https://github.com/terrastruct/d2/pull/966)
- `border-radius` is supported on connections (ELK and TALA only, since dagre uses curves). [#913](https://github.com/terrastruct/d2/pull/913)

#### Improvements 🧹

- Code blocks adapt to dark mode [#971](https://github.com/terrastruct/d2/pull/971)
- SVGs are fit to top left by default to avoid issues with zooming. [#954](https://github.com/terrastruct/d2/pull/954)
- Person shapes have labels below them and don't need to expand as much. [#960](https://github.com/terrastruct/d2/pull/960)

#### Bugfixes ⛑️

- Fixes a regression where PNG backgrounds could be cut off in the appendix. [#941](https://github.com/terrastruct/d2/pull/941)
- Fixes zooming not working in watch mode. [#944](https://github.com/terrastruct/d2/pull/944)
- Fixes insufficient vertical padding in dagre with direction: right/left. [#973](https://github.com/terrastruct/d2/pull/973)
