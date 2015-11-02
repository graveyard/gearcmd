TaskWrapper
=============

The taskwrapper is a program that handles the Gearman logic for any task-style program we
run so that the tasks themselves can be Gearman agnostic.

Interface
---------

The workerwrapper expects the tasks it runs to implement the following interface:

Input

 - The input arguments specified in the Gearman payload. For example if a job was submitted with the following payload:
"-h localhost -p 27017 -f s3_path". It would be translated into the corresponding command line arguments.

Output

 - The worker's "response" is the stdout of the process. This corresponds to the Gearman data field.
 - The worker's "warnings" are the last X lines of the stderr of the process. This corresponds to the Gearman warnings field.
 - The success / failure of the worker is a function of the exit code of the process.
 - Logs should be written to stderr.


Usage
-----
```
taskwrapper --name name --cmd cmd --gearman-host 'localhost' --gearman-port '4730'
```

Params:

- `name`: The name of the Gearman function to listen for
- `cmd`: The cmd to run when the wrapper receives a Gearman job
- `gearman-host` (optional): The Gearman host to connect to
- `gearman-port` (optional): The Gearman port to connect to


Example
-------
This will walk you through running a simple task through the taskwrapper.

First build the TaskWrapper and add it to your GOPATH with the command:
```
go get github.com/Clever/baseworker-go/cmd/taskwrapper`
```

Then create a bash script, test.sh. This will be the command the taskwrapper runs:
```
#!/bin/bash
echo $1
echo $2
```

Start the taskwrapper and tell it to run test.sh when it gets 'test' jobs.
```
taskwrapper --name test --cmd test.sh
```

At this point you should be able to submit Gearman jobs. For example, this command:
```
gearman -f test -h localhost -v -s "firstLine secondLine"
```
should output:
```
firstLine
secondLine
```
