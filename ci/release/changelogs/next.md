Dark mode support has landed! Thanks to https://github.com/vfosnar for such a substaintial first-time contribution to D2. Only one dark theme option accompanies the support, so if you have a dark theme you like, please feel free to submit into D2!

https://user-images.githubusercontent.com/3120367/221057628-e474b040-4ecb-4177-bb81-a04c95a4648f.mp4

D2 is now usable in non-Latin languages (and emojis!), as the font-measuring accounts for multi-byte characters. Thanks https://github.com/bo-ku-ra for keeping this top of mind.

D2 0.2.0 vs 0.2.1:

<img width="1200" alt="japanese" src="https://user-images.githubusercontent.com/3120367/221058010-9a405cbf-a1dc-4005-8820-bf17d920105c.png">

Sketch mode's subtle hand-drawn texture adapts to background colors. Previously the streaks were too subtle on lighter backgrounds and too prominent on darker ones.

<img width="399" alt="sketch" src="https://user-images.githubusercontent.com/3120367/221042548-aee58a6c-e0c0-4e58-8d79-d0b609a9d750.png">

This release also fixes a number of non-trivial layout bugs made in v0.2.0, and has better error messages. 

#### Features 🚀

- Dark theme support! See [docs](https://d2lang.com/tour/themes#dark-theme). [#613](https://github.com/terrastruct/d2/pull/613)
- Many non-Latin languages (e.g. Chinese, Japanese, Korean) are usable now that multi-byte characters are measured correctly. [#817](https://github.com/terrastruct/d2/pull/817)
- Dimensions can be set on containers (layout engine dependent). [#845](https://github.com/terrastruct/d2/pull/845)

#### Improvements 🧹

- Sketch mode's subtle hand-drawn texture adapts to background colors. [#613](https://github.com/terrastruct/d2/pull/613)
- Improves label legibility for dagre containers by stopping container edges early if they would run into the label. [#880](https://github.com/terrastruct/d2/pull/880)
- Cleaner watch mode logs without timestamps. [#830](https://github.com/terrastruct/d2/pull/830)
- Remove duplicate success logs in watch mode. [#830](https://github.com/terrastruct/d2/pull/830)
- CLI reports when a feature is incompatible with layout engine, instead of silently ignoring. [#845](https://github.com/terrastruct/d2/pull/845)
- `near` key set to direct parent or ancestor throws an appropriate error message. [#851](https://github.com/terrastruct/d2/pull/851)
- Dimensions and positions are able to be set from API. [#853](https://github.com/terrastruct/d2/pull/853)

#### Bugfixes ⛑️

- Fixes edge case where layouts with dagre show a connection from the bottom side of shapes being slightly disconnected from the shape. [#820](https://github.com/terrastruct/d2/pull/820)
- Bounding boxes weren't accounting for icons placed on the boundaries. [#879](https://github.com/terrastruct/d2/pull/879)
- Sequence diagrams using special characters in object IDs could cause rendering bugs. [#856](https://github.com/terrastruct/d2/issues/856)
- Fixes rare compiler bug when using underscores in edges to create objects across containers. [#824](https://github.com/terrastruct/d2/pull/824)
- Fixes rare possibility of rendered connections being hidden or cut off. [#828](https://github.com/terrastruct/d2/pull/828)
- Creating nested children within `sql_table` and `class` shapes are now prevented (caused confusion when accidentally done). [#834](https://github.com/terrastruct/d2/pull/834)
- Fixes graph deserialization bug. [#837](https://github.com/terrastruct/d2/pull/837)
- `steps` with non-map fields could cause panics. [#783](https://github.com/terrastruct/d2/pull/783)
