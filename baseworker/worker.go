package baseworker

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	gearmanWorker "github.com/Clever/gearman-go/worker"
	"gopkg.in/Clever/kayvee-go.v3/logger"
	"gopkg.in/eapache/go-resiliency.v1/retrier"
)

var (
	lg = logger.New("gearcmd")
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
	sync.Mutex
	fn   gearmanWorker.JobFunc
	name string
	w    *gearmanWorker.Worker
}

// Listen starts listening for jobs on the specified host and port.
func (worker *Worker) Listen(host, port string) error {
	if host == "" || port == "" {
		return errors.New("must provide host and port")
	}
	worker.w.AddServer("tcp4", fmt.Sprintf("%s:%s", host, port))
	worker.w.AddFunc(worker.name, worker.fn, gearmanWorker.Unlimited)
	if err := worker.w.Ready(); err != nil {
		lg.CriticalD("worker-error", logger.M{"error": err.Error()})
		os.Exit(1)
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

// Shutdown blocks while waiting for all jobs to finish
func (worker *Worker) Shutdown() {
	worker.Lock()
	defer worker.Unlock()
	lg.InfoD("shutdown", logger.M{"message": "Received sigterm. Shutting down gracefully."})
	if worker.w != nil {
		// Shutdown blocks, waiting for all jobs to finish
		worker.w.Shutdown()
	}
}

func defaultErrorHandler(e error) {
	lg.InfoD("gearman-error", logger.M{"error": e.Error()})
	if opErr, ok := e.(*net.OpError); ok {
		if !opErr.Temporary() {
			proc, err := os.FindProcess(os.Getpid())
			if err != nil {
				lg.CriticalD("err-getpid", logger.M{"error": err.Error()})
			}
			if err := proc.Signal(os.Interrupt); err != nil {
				lg.CriticalD("err-interrupt", logger.M{"error": err.Error()})
			}
		}
	}
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
		// Try to reconnect if it is a disconnect error
		wdc, ok := e.(*gearmanWorker.WorkerDisconnectError)
		if ok {
			lg.InfoD("err-disconnected-and-reconnecting", logger.M{"name": name, "error": e.Error()})
			r := retrier.New(retrier.ExponentialBackoff(5, 200*time.Millisecond), nil)
			if rcErr := r.Run(wdc.Reconnect); rcErr != nil {
				lg.CriticalD("err-disconnected-fully", logger.M{"name": name, "error": rcErr.Error()})
				defaultErrorHandler(rcErr)
				return
			}
			lg.InfoD("gearman-reconnected", logger.M{"name": name})
		} else {
			defaultErrorHandler(e)
		}
	}
	worker := &Worker{
		fn:   jobFunc,
		name: name,
		w:    w,
	}
	return worker
}
