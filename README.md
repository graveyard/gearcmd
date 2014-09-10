# gearcmd

`gearcmd` wraps command line programs to turn them into Gearman workers.

## Motivation

TODO

## Usage

TODO

## Installation

Install from source via `go get github.com/Clever/gearcmd`, or download a release on the [releases](https://github.com/Clever/gearcmd/releases) page.

## Local Development

Set this repository up in the [standard location](https://golang.org/doc/code.html) in your `GOPATH`, i.e. `$GOPATH/src/github.com/Clever/gearcmd`.
Once this is done, `make test` runs the tests.

### Rolling an official release

The release process requires a cross-compilation toolchain.
[`gox`](https://github.com/mitchellh/gox) can install the toolchain with one command: `gox -build-toolchain`.
From there you can build release tarballs for different OS and architecture combinations with `make release`.


Official releases are listed on the [releases](https://github.com/Clever/gearcmd/releases) page.
Steps to create an official release:

1. Rebase your feature branch on master.

2. Make a commit that bumps the version in the `VERSION` file. Tag this commit with the version as well: `git tag vX.Y.X`.
See [http://semver.org/](http://semver.org/) for how to determine what version change you should make for your changes.
[gitsem](https://github.com/clever/gitsem) is a command that can help with this step.

3. Push the version change commit and tag to Github: `git push origin --tags`, and, assuming it's been signed off on, merge your pull request.
Assuming you've rebased, this should be a fast-forward merge, and should not create a merge commit.
Check that the tagged commit created above is indeed the final commit in master.

4. Switch to master locally (`git checkout master && git pull`) and run `scripts/release_github`, passing in the required env:
    ```
    GITHUB_TOKEN=x GITHUB_REPO_USER=Clever GITHUB_REPO_NAME=gearcmd  scripts/release_github
    ```
