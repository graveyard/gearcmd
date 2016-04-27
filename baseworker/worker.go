package baseworker

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	gearmanWorker "github.com/Clever/gearman-go/worker"
	"gopkg.in/Clever/kayvee-go.v3/logger"
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

func defaultSigtermHandler(worker *Worker) {
	lg.InfoD("shutdown", logger.M{"message": "Received sigterm. Shutting down gracefully."})
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
		lg.InfoD("gearman-error", logger.M{"error": e.Error()})
		if opErr, ok := e.(*net.OpError); ok {
			if !opErr.Temporary() {
				proc, err := os.FindProcess(os.Getpid())
				if err != nil {
					lg.InfoD("err-getpid", logger.M{"error": err.Error()})
				}
				if err := proc.Signal(os.Interrupt); err != nil {
					lg.InfoD("err-interrupt", logger.M{"error": err.Error()})
				}
			}
		}
	}
	worker := &Worker{
		fn:             jobFunc,
		name:           name,
		w:              w,
		sigtermHandler: defaultSigtermHandler,
	}
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	go func() {
		<-sigc
		worker.Lock()
		defer worker.Unlock()
		worker.sigtermHandler(worker)
	}()
	return worker
}
