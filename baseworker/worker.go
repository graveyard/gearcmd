package baseworker

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
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

// GetJobID returns the job_id from the job handle
func GetJobID(j Job) string {
	splits := strings.Split(j.Handle(), ":")
	return splits[len(splits)-1]
}

// SigtermHandler is the definition for the function called after the worker receives
// a TERM signal.
type SigtermHandler func(*Worker)

// Worker represents a Gearman worker.
type Worker struct {
	sync.Mutex
	fn   gearmanWorker.JobFunc
	name string // this is also known as "function"
	w    *gearmanWorker.Worker
}

// Listen starts listening for jobs on the specified host and port.
func (worker *Worker) Listen(host, port string) error {
	if host == "" || port == "" {
		return errors.New("must provide host and port")
	}
	worker.w.AddServer("tcp4", fmt.Sprintf("%s:%s", host, port))
	worker.w.AddFunc(worker.name, worker.fn, gearmanWorker.Unlimited)

	worker.w.Lock()
	if err := worker.w.Ready(); err != nil {
		lg.CriticalD("worker-error", logger.M{
			"job_id":   worker.w.Id,
			"function": worker.name,
			"error":    err.Error()})
		os.Exit(1)
	}
	worker.w.Unlock()

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

	lg.InfoD("shutdown", logger.M{
		"message":  "Received sigterm. Shutting down gracefully.",
		"job_id":   worker.w.Id,
		"function": worker.name,
	})
	if worker.w != nil {
		// Shutdown blocks, waiting for all jobs to finish
		worker.w.Shutdown()
	}
}

func defaultErrorHandler(functionName string, e error) {
	lg.InfoD("gearman-error", logger.M{
		"function": functionName,
		"error":    e.Error()})
	if opErr, ok := e.(*net.OpError); ok {
		if !opErr.Temporary() {
			proc, err := os.FindProcess(os.Getpid())
			if err != nil {
				lg.CriticalD("err-getpid", logger.M{
					"function": functionName,
					"error":    err.Error()})
			}
			if err := proc.Signal(os.Interrupt); err != nil {
				lg.CriticalD("err-interrupt", logger.M{
					"function": functionName,
					"error":    err.Error()})
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
			lg.InfoD("err-disconnected-and-reconnecting", logger.M{
				"function": name,
				"error":    e.Error()})
			r := retrier.New(retrier.ExponentialBackoff(5, 200*time.Millisecond), nil)
			if rcErr := r.Run(wdc.Reconnect); rcErr != nil {
				lg.CriticalD("err-disconnected-fully", logger.M{
					"function": name,
					"error":    rcErr.Error()})
				defaultErrorHandler(name, rcErr)
				return
			}
			lg.InfoD("gearman-reconnected", logger.M{"function": name})
		} else {
			defaultErrorHandler(name, e)
		}
	}
	worker := &Worker{
		fn:   jobFunc,
		name: name,
		w:    w,
	}
	return worker
}
