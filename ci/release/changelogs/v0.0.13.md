#### Features 🚀

- Ability to export to PNG.

#### Improvements 🔧

- There is now equivalency between CLI flags and environment variables. For any option,
  you can set either the flag or its equivalent environment variable (flags take
  precedence). See `d2 --help` for more.
- Install script now uses `brew` for macOS machines with `brew` installed.

#### Bugfixes 🔴

- Labels for Image shapes in dagre and ELK are now placed above the shape, to not overlap
  with the actual image.
