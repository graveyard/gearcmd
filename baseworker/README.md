# NOTE

This is a copy of the deprecated `baseworker-go` repo.



# baseworker
--
    import "github.com/Clever/baseworker-go"

Package baseworker provides a simple wrapper around a Gearman worker, based on
http://godoc.org/github.com/mikespook/gearman-go.


### Example

Here's an example program that just listens for "test" jobs and logs the data
that it receives:

    package main

    import(
    	"github.com/Clever/baseworker-go"
    	"log"
    )

    func jobFunc(job baseworker.Job) ([]byte, error) {
    	log.Printf("Got job with data %s", job.Data())
    	return []byte{}, nil
    }

    func main() {
    	worker := baseworker.NewWorker("test", jobFunc)
    	defer worker.Close()
    	worker.Listen("localhost", "4730")
    }

## Usage

#### type Job

```go
type Job gearmanWorker.Job
```

Job is an alias for http://godoc.org/github.com/mikespook/gearman-go/worker#Job.

#### type JobFunc

```go
type JobFunc func(Job) ([]byte, error)
```

JobFunc is a function that takes in a Gearman job and does some work on it.

#### type SigtermHandler

```go
type SigtermHandler func(*Worker)
```

SigtermHandler is the definition for the function called after the worker
receives a TERM signal.

#### type Worker

```go
type Worker struct {
}
```

Worker represents a Gearman worker.

#### func  NewWorker

```go
func NewWorker(name string, fn JobFunc) *Worker
```
NewWorker creates a new gearman worker with the specified name and job function.

#### func (*Worker) Close

```go
func (worker *Worker) Close()
```
Close closes the connection.

#### func (*Worker) Listen

```go
func (worker *Worker) Listen(host, port string) error
```
Listen starts listening for jobs on the specified host and port.

## Testing

You can run the test cases by typing `make test`

## Documentation

The documentation is automatically generated via [godocdown](https://github.com/robertkrimen/godocdown).

You can update it by typing `make docs`.

They're also viewable online at [![GoDoc](https://godoc.org/github.com/Clever/baseworker-go?status.png)](https://godoc.org/github.com/Clever/baseworker-go).
