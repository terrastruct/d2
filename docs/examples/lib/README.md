# D2 library examples

We have a few examples in this directory on how to use the D2 library to turn D2 scripts
into rendered svg diagrams and more.

Each example is runnable though does not include error handling for readability.

### [./1-d2lib](./1-d2lib)

A minimal example showing you how to compile the diagram `x -> y` into an svg.

### [./2-d2oracle](./2-d2oracle)

D2 is built to be hackable -- the language has an API built on top of it to make edits
programmatically.

Modifying the previous example, this example demonstrates how
[d2oracle](../../../d2oracle) can be used to create a new shape, style it programatically
and then output the modified d2 script.

This makes it easy to build functionality on top of D2. Terrastruct uses the
[d2oracle](../../../d2oracle) API to implement editing of D2 from mouse actions in a
visual interface.

### [./3-lowlevel](./3-lowlevel)

`d2lib` from the first example is just a wrapper around the lower level APIs. They
can be used directly and this example demonstrates such usage.

This shouldn't be necessary for most usecases.
