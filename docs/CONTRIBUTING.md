# Contributing

<!-- toc -->
- <a href="#welcome" id="toc-welcome">Welcome</a>
- <a href="#ci" id="toc-ci">CI</a>
- <a href="#logistics" id="toc-logistics">Logistics</a>
- <a href="#dev" id="toc-dev">Dev</a>
  - <a href="#content" id="toc-content">Content</a>
  - <a href="#tests" id="toc-tests">Tests</a>
  - <a href="#documentation" id="toc-documentation">Documentation</a>
  - <a href="#questions" id="toc-questions">Questions</a>

## Welcome

D2's [long-term mission](https://d2lang.com/tour/future/) is to significantly reduce the
amount of time and effort it takes to create and maintain high-quality diagrams for every
software team. We started this because we love the idea of creating diagrams with text --
but it was clear the existing solutions were inadequete in their state and speed of
execution for this idea to be mainstream.

We've tried our best to avoid the mistakes of the past and take inspiration from the most
successful modern programming and configuration languages.

D2 has built up each step of the text-to-diagram pipeline from scratch, rethinking each
one from first principles, from the dead simple syntax, to the readable compiler, our own
SVG renderer, etc.

D2 is committed to making something people want to use. That means contributions don't
only have to be in the form of pull requests. Your bug reports, plugins, examples,
discussions of new ideas, help a great deal.

If you'd like to get involved, we're also committed to helping you merge that first
pull request. You should be able to freely pick up Issues tagged as "good first issue". If
you need help getting started, please don't hesitate to pop into Discord -- if you want to
help, I'm sure we'll find the perfect task (complexity matches your appetite and
programming experience, in an area you're interested in, etc).

## CI

Most of D2's CI is open sourced in its own
[repository](https://github.com/terrastruct/ci).

`./make.sh` runs everything. Subcommands to run individual parts of the CI:

- `./make.sh fmt`
- `./make.sh lint`
- `./make.sh test`
- `./make.sh race`
- `./make.sh build`


Please make sure CI is passing for any PRs submitted for review.

Most of the CI scripts rely on a submodule shared between many D2 repositories:
[https://github.com/terrastruct/ci](https://github.com/terrastruct/ci). You should fetch
the submodule whenever it differs:

```sh
git submodule update --recursive
```

If running for the first time for a repo (e.g. new clone), add `--init`:

```sh
git submodule update --init --recursive
```

## Logistics

- Use Go 1.18. Go 1.19's autofmt inexplicably strips spacing from ASCII art in comments.
  We're working on it.
- Please sign your commits
  ([https://github.com/terrastruct/d2/pull/557#issuecomment-1367468730](https://github.com/terrastruct/d2/pull/557#issuecomment-1367468730)).
- D2 uses Issues as TODOs. No auto-closing on staleness.
- Branch off `master`.
- If there's an Issue related, include it by adding "[one-word] #[issue]", e.g. "Fixes
  #123" somewhere in the description.
- Whenever possible and relevant, include a screenshot or screen-recording.

## Dev

### Content

Unless you've contributed before, it's safest to choose an Issue with a "good first issue"
label. If you'd like to propose new functionality or change to current functionality,
please create an Issue first with a proposal. When you start work on an Issue, please
leave a comment so others know that it's being worked on.

### Tests

D2 has extensive tests, and all code changes must include tests.

With the exception of changes to the renderer, all code should include a package-specific
test. If it's a visual change, an end-to-end (e2e) test should accompany.

Let's say I make some code changes. I can easily see how this affects the end result by
running:

```
./ci/e2ereport.sh -delta
```

This gives me a nice HMTL output of what the test expected vs what it got:

![screencapture-file-Users-alexanderwang-dev-alixander-d2-e2etests-out-e2e-report-html-2023-02-14-10_15_07](https://user-images.githubusercontent.com/3120367/218822836-bcc517f2-ae3e-4e0d-83f6-2cbaa2fd9275.png)

If you're testing labels and strings, it's encouraged to use 1-letter strings (`x`) in small
functional tests to minimally pinpoint issues. If you are testing something that exercises
variations in strings, or want to mimic more realistic diagram text, it's encouraged you
generate random strings and words from `fortune`. It gives a good range of the English
language. (Sometimes it gives controversial sentences -- don't use those.)

Script to generate one line of random text:
```
ipsum1() {
  fortune | head -n1 | sed 's/^ *//;s/ *$//' | tr -d '\n' | tee /dev/stderr | pbcopy
}
```

#### Running tests

Run: `./ci/test.sh`

CI runs tests with `-race` to catch potential race conditions. It's much slower, but if
your machine can run it locally, you can do so with `./make.sh race`.

If you add a new test and run, it will show failure. That's because the vast majority of
D2's tests are comparing outputs. You don't define the expected output manually. The
testing library generates it and it's checked into version control if it looks right. So
for the first run of a new test, it has no expected output, and will fail. To accept the
  result as the expected, run the test with environment variable `TESTDATA_ACCEPT=1`.

#### Chaos tests

D2 has [chaos tests](https://en.wikipedia.org/wiki/Chaos_engineering) which produce random
configurations of diagrams. It can be helpful to run a few iterations (N~=100) to spot
cover your manual tests.

`D2_CHAOS_MAXI=100 D2_CHAOS_N=100 ./ci/test.sh ./d2chaos`

### Documentation

The code itself should be documented as appropriate with Go-style comments. No rules here,
`GetX()` doesn't need a `// GetX gets X`.

If it's some new functionality, please submit a pull request to document it in the
language docs:
[https://github.com/terrastruct/d2-docs](https://github.com/terrastruct/d2-docs).

### Questions

If you have any questions or would like to get more involved, feel free to open an issue
to discuss publicly, or chat in [Discord](https://discord.gg/NF6X8K4eDq)! If you have a
private inquiry, feel free to email us at hi@d2lang.com.
