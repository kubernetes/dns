# Managing go dependencies

Go dependencies are managed using
the [dep tool](https://github.com/golang/dep).

```text
USAGE

Add a new package:

  # Add import reference of acme.com/widget.
  $ vi pkg/foo.go

  # Add dependency using dep ensure command
  $ dep ensure
  
  Locking to a specific commit of a repo:
  Add a "constraint" to Gopkg.toml specifying the version/revision needed
```
More help [here](https://golang.github.io/dep/docs/introduction.html)
