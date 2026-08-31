# e2e — the tool against a real project

`project/` is an openrecord project of the shape a real one has: four declared surfaces, nine decisions
across seven concerns, a subgroup in each store, a pending record, a deliberate-absence record, and a
capability spec of every one of the four types.

It exists because the unit tests each build the smallest store that makes their own assertion pass, and
a fixture that small has no room for the cases that actually break. A renderer that dropped every line
of wrapped prose, and one that quoted a state name twice, both stayed green through a full suite — no
unit fixture had a step long enough to wrap, or a state name with a space in it. This one does.

## Layout

```
e2e/
  bodies/     the record bodies, fed to `record write --body-file`
  project/    the fixture: a source tree plus the .openrecord store
  seed.sh     the commands that produce project/.openrecord
  *_test.go   the suite, driving the built binary against a copy of project/
```

## The fixture is output, never source

`project/.openrecord` is checked in, but nothing in it is hand-written: it is whatever `seed.sh`
produces. Edit `bodies/` or `seed.sh`, re-run `./e2e/seed.sh`, and commit the result.

`TestFixtureIsWhatTheCommandsProduce` runs `./seed.sh --check`, which rebuilds into a temporary
directory and diffs. A hand-edit to the store fails there — otherwise the fixture could drift into a
shape the tool would never write, and every test reading it would be testing a file rather than a
command.

## The fixture is valid, and stays that way

`TestFixtureIsAValidStore` asserts `validate` exits 0 and returns exactly three findings, all warnings,
all on the order lifecycle: the state an order starts in is never the target of a transition, and the
two it ends in are never a source. Those are true of any correct state machine, and keeping them is
deliberate — it is the only place the warning path runs against a record somebody would plausibly write.

A test that needs a broken store breaks its **own copy**. `project(t)` hands out a fresh copy in a
temporary directory, so nothing a test does reaches the checked-in fixture.

## What the suite drives

The built binary, not the packages. Exit status and the split between stdout and stderr are part of
what openrecord promises, and both are wiring that only exists once `cmd/openrecord` is assembled.
`TestMain` builds it once.

The JSON shapes are re-declared in `harness_test.go` rather than imported from `internal/`. A test that
shares the producer's struct cannot notice a field being renamed — which is precisely what a consumer
would notice first.
