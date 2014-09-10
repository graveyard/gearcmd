# gearcmd

`gearcmd` wraps command line programs and turns them into Gearman workers.

## Motivation

Instead of writing programs *as* [Gearman](http://gearman.org/) workers that speak the [Gearman protocol](http://gearman.org/protocol/), you can write programs as regular Unix command line programs. `gearcmd` will speak Gearman on behalf of your program and translate it into the standard command line protocol.

This allows programs to be easily run locally in a Unix dev environment or in other contexts besides Gearman.

In addition, this decouples your program's process from the Gearman worker's process, ensuring that if your process crashes, the Gearman worker won't, so it will be able to report the crash as a job failure. By default, if a worker crashes, Gearman will silently retry jobs with a new worker without reporting the crash. `gearcmd` ensures that crashes result in loud failures that you can address.

## Usage

    gearcmd -name <function name> -cmd <command> -gearman-host <host> -gearman-port <port>

Params:

- `name`: The name of the Gearman function to listen for
- `cmd`: The command to run when the wrapper receives a Gearman job
- `gearman-host` (optional): The Gearman host to connect to. Defaults to `localhost`.
- `gearman-port` (optional): The Gearman port to connect to. Defaults to `4730`.

### Command Interface

#### Input

The command will be given as its arguments the exact arguments passed as the Gearman payload. These arguments will be parsed as if the command were being called in Bash. For example, running `gearcmd --name grep --cmd grep` and then submitting a Gearman job with function `grep` and payload `-i 'some regex' some-file.txt` would result in `grep` being run as if it were called on the command line like so: `grep -i 'some regex' some-file.txt`.

#### Output

- The command's stdout will be emitted as the Gearman worker's `WORK_DATA` events.
- The last 5 lines of the command's stderr will be emitted as the Gearman worker's `WORK_WARNING` events.
- If the command has exit code 0, the Gearman worker will emit `WORK_COMPLETE`, otherwise it will emit `WORK_FAIL`.
- The command's stderr will be logged in `gearcmd`'s stderr.

### Example

This will walk you through running a simple task through `gearcmd`.

First build `gearcmd` and add it to your GOPATH with the command:

    go get github.com/Clever/baseworker-go/cmd/gearcmd`

Then create a bash script, `my-echo.sh`. This will be the command `gearcmd` runs:

    #!/bin/bash
    echo $1
    echo $2

Run a local Gearman server:

    gearmand -d

Start `gearcmd` and tell it to register itself as a Gearman worker for jobs with function `echo`, and run `my-echo.sh` to process these jobs:

    gearcmd -name echo -cmd my-echo.sh

At this point you should be able to submit Gearman jobs. For example, this command:

    gearman -f echo -h localhost -v -s "firstLine secondLine"

should output:

    firstLine
    secondLine

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
