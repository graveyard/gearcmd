package baseworker

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	gearmanWorker "github.com/Clever/gearman-go/worker"
)

// JobFunc is a function that takes in a Gearman job and does some work on it.
type JobFunc func(Job) ([]byte, error)

// Job is an alias for http://godoc.org/github.com/mikespook/gearman-go/worker#Job.
type Job gearmanWorker.Job

// SigtermHandler is the definition for the function called after the worker receives
// a TERM signal.
type SigtermHandler func(*Worker)

// Worker represents a Gearman worker.
type Worker struct {
	fn             gearmanWorker.JobFunc
	name           string
	w              *gearmanWorker.Worker
	sigtermHandler SigtermHandler
}

// Listen starts listening for jobs on the specified host and port.
func (worker *Worker) Listen(host, port string) error {
	if host == "" || port == "" {
		return errors.New("must provide host and port")
	}
	worker.w.AddServer("tcp4", fmt.Sprintf("%s:%s", host, port))
	worker.w.AddFunc(worker.name, worker.fn, gearmanWorker.Unlimited)
	if err := worker.w.Ready(); err != nil {
		log.Fatal(err)
	}
	worker.w.Work()
	return nil
}

// Close closes the connection.
func (worker *Worker) Close() {
	if worker.w != nil {
		worker.w.Close()
	}
}

func defaultSigtermHandler(worker *Worker) {
	log.Println("Received sigterm. Shutting down gracefully.")
	if worker.w != nil {
		// Shutdown blocks, waiting for all jobs to finish
		worker.w.Shutdown()
	}
	os.Exit(0)
}

// NewWorker creates a new gearman worker with the specified name and job function.
func NewWorker(name string, fn JobFunc) *Worker {
	// Turn a JobFunc into gearmanWorker.JobFunc
	jobFunc := func(job gearmanWorker.Job) ([]byte, error) {
		castedJob := Job(job)
		return fn(castedJob)
	}
	w := gearmanWorker.New(gearmanWorker.OneByOne)
	w.ErrorHandler = func(e error) {
		log.Println(e)
		if opErr, ok := e.(*net.OpError); ok {
			if !opErr.Temporary() {
				proc, err := os.FindProcess(os.Getpid())
				if err != nil {
					log.Println(err)
				}
				if err := proc.Signal(os.Interrupt); err != nil {
					log.Println(err)
				}
			}
		}
	}
	worker := &Worker{fn: jobFunc, name: name, w: w, sigtermHandler: defaultSigtermHandler}
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	go func() {
		<-sigc
		worker.sigtermHandler(worker)
	}()
	return worker
}
