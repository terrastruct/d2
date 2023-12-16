D2 0.6.3 allows you to make your own and customize existing D2 themes. Here's an example with some random color codes.

<img width="321" alt="Screen Shot 2023-12-16 at 3 13 04 PM" src="https://github.com/terrastruct/d2/assets/3120367/106fbb18-4650-44ac-bb66-359ef99dca3b" />

> See [docs](https://d2lang.com/tour/themes/#special-themes)

> [Playground link](https://play.d2lang.com/?script=rJLNjqM6EIX3fopS7joo4S8KVxqJBHiNkYVrwIpjW7aTTNTi3UcGDEn3SL2ZBQt_Ltepc4o7NbaADwLA4m2r5C_eTUcA1-MVt-qOxnCGNmCA076AzX9xfaiSeBNY7Fmen055ubDEszKr8vKwsNSzc1bvy2xhmWd1XtfH88Jyz5qmyY7VhsywLEeVXZUeyv1mgWPLNC6zJlvh2PO4O5fNcX0_qTdpmiT5UjrJN825qmY7A_HfQCS6hzKXyXmLQoBTDzQhCUsdCsHdaza2pxoLsE4ZZD8ZdTRcuKfA6HoTjmuBBThzw1lsSttQaa_cOTTkU3fY_ni9LsCiZP-wZpxAScElglbGURH83HgBH7OlHn_TTskhPPDeQBvVorVqzcQpQzv8Ekj7FFwyNN-nMbdf445eZvYW3oWjRVH3SiII1VkyEHKzYVHzBBqNVZIAPDhzfQH7ZBfqfNd519Hrmq_0gtBSIb5UvaUV-Zho6-eZBEdz1hl1wS2jti8g8VJUc7Bo7t-1YtxqQZ-f6r2xyQW3jkynsBtNO_z_r6EOJPzF0XtuvuUqQP4EAAD__w%3D%3D&layout=elk&theme=1&)

#### Features 🚀

- Themes can be customized via `d2-config` vars. [#1777](https://github.com/terrastruct/d2/pull/1777)

#### Improvements 🧹

- Icons can be added for special objects (sql_table, class, code, markdown, latex). [#1774](https://github.com/terrastruct/d2/pull/1774)

#### Bugfixes ⛑️

- Fix importing files that override an existing value with an array. [#1762](https://github.com/terrastruct/d2/pull/1762)
- Fixes missing unfilled triangle arrowheads when sketch flag is on. [#1763](https://github.com/terrastruct/d2/pull/1763)
- Fixes a bug where the render target could be incorrect if the target path contains "index". [#1764](https://github.com/terrastruct/d2/pull/1764)
- Fixes ELK layout with outside labels/icons. [#1776](https://github.com/terrastruct/d2/pull/1776)
- Fixes a bug where an edge could become disconnected with dagre layout and direction right. [#1778](https://github.com/terrastruct/d2/pull/1778)
