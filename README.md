# gearcmd

`gearcmd` wraps command line programs and turns them into Gearman workers.

## Motivation

Instead of writing programs *as* [Gearman](http://gearman.org/) workers that speak the [Gearman protocol](http://gearman.org/protocol/), you can write programs as regular Unix command line programs. `gearcmd` will speak Gearman on behalf of your program and translate it into the standard command line protocol.

This allows programs to be easily run locally in a Unix dev environment or in other contexts besides Gearman.

In addition, this decouples your program's process from the Gearman worker's process, ensuring that if your process crashes, the Gearman worker won't, so it will be able to report the crash as a job failure. By default, if a worker crashes, Gearman will silently retry jobs with a new worker without reporting the crash. `gearcmd` ensures that crashes result in loud failures that you can address.

While the [gearman command line tool](http://gearman.info/bin/gearman.html) offers similar functionality, `gearcmd` differs in a few key ways:
- `gearcmd` lets clients submit different command line flags for each job, while `gearman` passes the job payload through the command's stdin.
- While both forward the command's stderr to the worker's stderr, `gearcmd` also emits stderr output as worker warnings, allowing clients to receive error information from workers.

## Usage

    gearcmd -name <function name> -cmd <command> -host <host> -port <port>

Params:

- `name`: The name of the Gearman function to listen for.
- `cmd`: The command to run when the wrapper receives a Gearman job.
- `host` (optional): The Gearman host to connect to. Defaults to `$GEARMAN_HOST`.
- `port` (optional): The Gearman port to connect to. Defaults to `$GEARMAN_PORT`.
- `parseargs` (optional): If false, send the job payload directly to the cmd as its first argument without parsing it. Requires flag syntax `-parseargs=[true/false]`. It will not work properly without the equal sign.
- `cmdtimeout` (optional): Maximum time for the command to run before it will be killed, as parsed by [time.ParseDuration](http://golang.org/pkg/time/#ParseDuration) (e.g. `2h`, `30m`, `2h30m`). Defaults to never.
- `retry` (optional): Number of times to retry the job if it fails. Defaults to 0.

### Command Interface

#### Input

The command will be given as its arguments the exact arguments passed as the Gearman payload. These arguments will be parsed as if the command were being called in Bash. For example, running `gearcmd --name grep --cmd grep` and then submitting a Gearman job with function `grep` and payload `-i 'some regex' some-file.txt` would result in `grep` being run as if it were called on the command line like so: `grep -i 'some regex' some-file.txt`.

#### Output

- The command's stdout will be emitted as the Gearman worker's `WORK_DATA` events.
- The last 5 lines of the command's stderr will be emitted as the Gearman worker's `WORK_WARNING` events.
- If the command has exit code 0, the Gearman worker will emit `WORK_COMPLETE`, otherwise it will emit `WORK_FAIL`.
- The command's stdout and stderr will be outputted to `gearcmd`'s stdout and stderr respectively.

### Example

This will walk you through running a simple task through `gearcmd`. First, install `gearcmd` as described [below](#Installation).

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

Install from source via `go get github.com/Clever/gearcmd/cmd/gearcmd`, or download a release on the [releases](https://github.com/Clever/gearcmd/releases) page.

## Local Development

Set this repository up in the [standard location](https://golang.org/doc/code.html) in your `GOPATH`, i.e. `$GOPATH/src/github.com/Clever/gearcmd`.
Once this is done, `make test` runs the tests.

The release process requires a cross-compilation toolchain.
[`gox`](https://github.com/mitchellh/gox) can install the toolchain with one command: `gox -build-toolchain`.
From there you can build release tarballs for different OS and architecture combinations with `make release`.

### Rolling an official release

Official releases are listed on the [releases](https://github.com/Clever/gearcmd/releases) page.
To create an official release:

1. On `master`, bump the version in the `VERSION` file in accordance with [semver](http://semver.org/).
You can do this with [`gitsem`](https://github.com/clever/gitsem), but make sure not to create the tag, e.g. `gitsem -tag=false patch`.

2. Push the change to Github. Drone will automatically create a release for you.
