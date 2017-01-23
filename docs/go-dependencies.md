# Managing go dependencies

Go dependencies are managed using
the [godep tool](https://github.com/tools/godep). `build/dep.sh` is an ease of
use wrapper around `godep`.

```text
dep.sh COMMAND ...

Manages godep dependencies. This mostly a wrapper around operations
involving godep and a Docker image containing a clean copy of the
dependencies.

image
  Create image with godep dependencies (k8s-dns-godep).

verify
  Verify that the godep matches the dependencies in the code. Exits
  with success if there are no differences.

enter [-u]
  Run Docker container interactively to manage godep. If -u is
  specified, then update the image with changes made.

save [PKG1 PKG2 ...]
  save PKGs as new dependencies. Note: the PKGs must be referenced
  from the code, otherwise godep will ignore the new packages.

-h|--help
  This help message.

USAGE

Add a new package:

  # Add import reference of acme.com/widget.
  $ vi pkg/foo.go

  # Add widget to the vendor directory. This should modify the godep
  # file appropriately.
  $ build/dep.sh save acme.com/widget


Updating a package:

  (It is recommended to do this manually due to the fragility of godep update)

  # Enter the container interactively.
  $ build/dep.sh enter -u

  # inside container
  $ rm -rf /src/$DEP # repo root
  $ go get $DEP/...
  # Change code in Kubernetes, if necessary.
  $ rm -rf Godeps
  $ rm -rf vendor
  $ ./build/dep.sh save
  $ exit

  $ git checkout -- $(git status -s | grep "^ D" | awk '{print $2}' | grep ^Godeps)

Verifying Godeps.json match:

  $ build/dep.sh verify
```

