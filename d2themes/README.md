# d2themes

`d2themes` defines themes for D2. You can add a new one in `./d2themescatalog`, give a
unique ID, and specify it in the CLI or library to see it.

For example, to use the "Shirley temple" theme, which has an ID of 102:

```sh
d2 --theme 102 --watch example-twitter.d2 out-twitter.svg
```

Run `d2 --help` or `man d2` for more.


# Themes overview

<img src="../docs/assets/themes_overview.png" />

# Color coding guide

<img src="../docs/assets/themes_coding.png" />

# Color coding example

<img src="../docs/assets/themes_coding_example.png" />

# Container gradients

To distinguish container nesting, objects get progressively lighter the more nested it is.

<img src="../docs/assets/themes_gradients.png" width="300px" />
