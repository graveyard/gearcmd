package gearcmd

import (
	"bufio"
	"bytes"
	"container/ring"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Clever/gearcmd/argsparser"
	"github.com/Clever/gearcmd/baseworker"
	"github.com/Clever/gearcmd/config"
	"gopkg.in/Clever/kayvee-go.v6/logger"
)

// TaskConfig defines the configuration for the task.
// Use constructor for a new struct
type TaskConfig struct {
	FunctionName            string
	FunctionCmd             string
	WarningLines            int
	ParseArgs               bool
	CmdTimeout              time.Duration
	RetryCount              int
	Halt                    chan struct{}
	LastResults             *ring.Ring
	ErrorResultsBackoffRate time.Duration
	SigtermGracePeriod      time.Duration
	// this variable tracks how much to backoff if another failure happens
	currentErrorResultsBackoff time.Duration
}

var (
	lg = logger.New("gearcmd")
	// legacy logger used to maintain existing alerts.
	// Once all workers are migrated to using gearcmd >= v0.5.0 and the alarms are switched over,
	// then we can remove this logger
	legacyLg = logger.New("gearman")
)

// ProcessWithErrorBackoff calls Process and sleeps if the last N jobs returned an error
func (conf *TaskConfig) ProcessWithErrorBackoff(job baseworker.Job) (b []byte, returnErr error) {
	b, returnErr = conf.Process(job)
	if conf.LastResults == nil || conf.ErrorResultsBackoffRate == 0 {
		return b, returnErr
	}

	conf.LastResults.Value = returnErr
	conf.LastResults = conf.LastResults.Next()

	// default to assume error and loop through last results to find any previously succesful jobs
	allPreviousJobsResultedInError := true
	conf.LastResults.Do(func(value interface{}) {
		if value == nil {
			// successful result
			allPreviousJobsResultedInError = false
		}
	})
	conf.currentErrorResultsBackoff = time.Duration(math.Min(float64(conf.currentErrorResultsBackoff), float64(60*time.Second)))
	if allPreviousJobsResultedInError {
		lg.WarnD("timeout-due-to-errors", logger.M{
			"error_count":      conf.LastResults.Len(),
			"backoff_duration": conf.currentErrorResultsBackoff,
		})
		config.Clock.Sleep(conf.currentErrorResultsBackoff)
		conf.currentErrorResultsBackoff = conf.currentErrorResultsBackoff * 2
	} else {
		conf.currentErrorResultsBackoff = conf.ErrorResultsBackoffRate
	}
	return b, returnErr
}

// Process runs the Gearman job by running the configured task.
// We need to implement the Task interface so we return (byte[], error)
// though the byte[] is always nil.
func (conf *TaskConfig) Process(job baseworker.Job) (b []byte, returnErr error) {
	jobID := getJobID(job)
	if jobID == "" {
		jobID = strconv.Itoa(rand.Int())
		lg.InfoD("rand-job-id", logger.M{"msg": "no job id parsed, random assigned."})
	}

	jobData := string(job.Data())
	data := logger.M{
		"function": conf.FunctionName,
		"job_id":   jobID,
		"job_data": jobData,
	}

	// This wraps the actual processing to do some logging
	lg.InfoD("START", data)
	start := time.Now()

	for try := 0; try < conf.RetryCount+1; try++ {
		// We create a temporary directory to be used as the work directory of the process.
		// A new work directory is created for every retry of the process.
		// We try to use MEOS_SANDBOX, the default will be the system temp directory.
		tempDirPath, err := ioutil.TempDir(os.Getenv("MESOS_SANDBOX"),
			fmt.Sprintf("%s-%s-%d-", conf.FunctionName, jobID, try))
		if err != nil {
			lg.CriticalD("tempdir-failure", logger.M{"error": err.Error()})
			return nil, err
		}
		defer os.RemoveAll(tempDirPath)

		// insert the job id and the work directory path into the environment
		extraEnvVars := []string{
			fmt.Sprintf("JOB_ID=%s", jobID),
			fmt.Sprintf("WORK_DIR=%s", tempDirPath)}

		err = conf.doProcess(job, extraEnvVars, try)
		end := time.Now()
		data["type"] = "gauge"

		// Return if the job was successful.
		if err == nil {
			lg.InfoD("SUCCESS", logger.M{
				"type":     "counter",
				"function": conf.FunctionName})
			legacyLg.InfoD("success", logger.M{
				"type":     "counter",
				"function": conf.FunctionName})
			data["value"] = 1
			data["success"] = true
			lg.InfoD("END", data)
			// Hopefully none of our jobs last long enough for a uint64...
			// Note that we cannot use lg.GaugeIntD because duration is uint64
			lg.InfoD("duration", logger.M{
				"value":    uint64(end.Sub(start).Seconds() * 1000),
				"type":     "gauge",
				"function": conf.FunctionName,
				"job_id":   jobID,
				"job_data": jobData})
			return nil, nil
		}

		data["value"] = 0
		data["success"] = false
		data["error_message"] = err.Error()
		returnErr = err

		if try != conf.RetryCount {
			lg.ErrorD("RETRY", data)
		}
	}

	lg.InfoD("FAILURE", logger.M{"type": "counter", "function": conf.FunctionName})
	legacyLg.InfoD("failure", logger.M{"type": "counter", "function": conf.FunctionName})
	lg.ErrorD("END", data)
	return nil, returnErr
}

// getJobID returns the jobId from the job handle
func getJobID(job baseworker.Job) string {
	splits := strings.Split(job.Handle(), ":")
	return splits[len(splits)-1]
}

func (conf *TaskConfig) doProcess(job baseworker.Job, envVars []string, tryCount int) error {
	defer func() {
		// If we panicked then set the panic message as a warning. Gearman-go will
		// handle marking this job as failed.
		if r := recover(); r != nil {
			err := r.(error)
			job.SendWarning([]byte(err.Error()))
		}
	}()

	// shutdownTicker will effectively control the executution of the ticker.
	shutdownTicker := make(chan interface{})
	defer func() {
		shutdownTicker <- 1
	}()

	// every minute we will output a heartbeat kayvee log for the job.
	tickUnit := time.Minute
	ticker := time.NewTicker(tickUnit)
	go func() {
		defer ticker.Stop()
		units := 0
		for {
			select {
			case <-shutdownTicker:
				close(shutdownTicker)
				return
			case <-ticker.C:
				units++
				lg.GaugeIntD("heartbeat", units, logger.M{
					"try_number": tryCount,
					"function":   job.Fn(),
					"job_id":     getJobID(job),
					"unit":       tickUnit.String(),
				})
			}
		}
	}()

	var args []string
	var err error
	if conf.ParseArgs {
		args, err = argsparser.ParseArgs(string(job.Data()))
		if err != nil {
			return fmt.Errorf("Failed to parse args: %s", err.Error())
		}
	} else {
		args = []string{string(job.Data())}
	}
	cmd := exec.Command(conf.FunctionCmd, args...)

	// insert provided env vars into the job
	cmd.Env = append(os.Environ(), envVars...)

	// create new pgid for this process so we can later kill all subprocess launched by it
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Write the stdout and stderr of the process to both this process' stdout and stderr
	// and also write it to a byte buffer so that we can return it with the Gearman job
	// data as necessary.
	var stderrbuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrbuf)
	defer sendStderrWarnings(&stderrbuf, job, conf.WarningLines)

	stdoutReader, stdoutWriter := io.Pipe()
	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutWriter)

	done := make(chan error)
	// Track when the job has started so that we don't try and sigterm a nil process
	started := make(chan struct{})
	go func() {
		defer close(done)

		finishedProcessingStdout := make(chan error)
		go func() {
			finishedProcessingStdout <- streamToGearman(stdoutReader, job)
		}()

		if err := cmd.Start(); err != nil {
			done <- err
			return
		}
		close(started)
		// Save the cmdErr. We want to process stdout and stderr before we return it
		cmdErr := cmd.Wait()
		stdoutWriter.Close()

		stdoutErr := <-finishedProcessingStdout
		if cmdErr != nil {
			done <- cmdErr
		} else if stdoutErr != nil {
			done <- stdoutErr
		}
	}()
	<-started

	timedOut := false
	defer func() {
		timedOutCount := 0
		if timedOut {
			timedOutCount = 1
		}
		lg.CounterD("worker-timed-out", timedOutCount, logger.M{
			"timeout":  conf.CmdTimeout,
			"function": conf.FunctionName,
		})
	}()

	// No timeout
	if conf.CmdTimeout == 0 {
		select {
		case err := <-done:
			// Will be nil if the channel was closed without any errors
			return err
		case <-conf.Halt:
			if err := stopProcess(cmd.Process, conf.SigtermGracePeriod); err != nil {
				return fmt.Errorf("error stopping process: %s", err)
			}
			return fmt.Errorf("killed process due to sigterm")
		}
	}
	select {
	case err := <-done:
		// Will be nil if the channel was closed without any errors
		return err
	case <-conf.Halt:
		if err := stopProcess(cmd.Process, conf.CmdTimeout); err != nil {
			return fmt.Errorf("error stopping process: %s", err)
		}
		return nil
	case <-time.After(conf.CmdTimeout):
		timedOut = true
		if err := stopProcess(cmd.Process, 0); err != nil {
			return fmt.Errorf("error timing out process after %s: %s", conf.CmdTimeout.String(), err)
		}
		return fmt.Errorf("process timed out after %s", conf.CmdTimeout.String())
	}
}

// stopProcess kills a given process. It's second argument is a grace period.
// If, after the grace period, the process hasn't exited, SIGKILL will be sent.
// It also calls os.Exit, since we currently rely on cutting off the connection
// with gearmand to trigger reassignment of work to another worker.
func stopProcess(p *os.Process, gracePeriod time.Duration) error {
	lg.InfoD("stopping-process", logger.M{"pid": p.Pid, "grace_period": gracePeriod})
	if err := p.Signal(os.Signal(syscall.SIGTERM)); err != nil {
		return fmt.Errorf("unable to send SIGTERM, error: %s", err)
	}
	timer := time.AfterFunc(gracePeriod, func() {
		// kill entire group of process spawned by our cmd.Process
		targetID := p.Pid
		pgid, err := syscall.Getpgid(p.Pid)
		if err != nil {
			lg.InfoD("unable-to-get-pgid", logger.M{"pid": p.Pid})
		} else {
			// minus sign required to kill PGIDs
			// https://linux.die.net/man/2/kill
			targetID = -pgid
		}
		lg.InfoD("killing-pid", logger.M{"pid": p.Pid, "target_id": targetID})
		syscall.Kill(targetID, syscall.SIGKILL)
	})
	lg.InfoD("waiting-pid", logger.M{"pid": p.Pid})
	pState, err := p.Wait()
	timer.Stop()
	if err != nil {
		if strings.Contains(err.Error(), "waitid: no child processes") {
			// process was reaped before the call to Wait(), probably by sigterm handling in run script
			lg.InfoD("process-exited-outside-of-gearcmd", logger.M{"pid": p.Pid, "code": -1})
			os.Exit(0)
		}
		lg.ErrorD("unknown-wait-err", logger.M{"pid": p.Pid, "wait-err": err.Error()})
		os.Exit(2)
	}
	status := pState.Sys().(syscall.WaitStatus)
	switch {
	case status.Exited() && status.ExitStatus() == 0:
		lg.InfoD("process-exited", logger.M{"pid": p.Pid, "code": status.ExitStatus()})
		os.Exit(0)
	case status.Signaled() && status.Signal() == syscall.SIGKILL:
		lg.ErrorD("process-killed", logger.M{"pid": p.Pid})
		// Use a distinctive exit code to communicate that the cmd did not
		// exit after receving SIGTERM
		os.Exit(2)
	default:
		lg.ErrorD("process-in-unknown-state", logger.M{"pid": p.Pid, "state": pState.String()})
		os.Exit(3)
	}
	return nil
}

// This function streams the reader to the Gearman job (through job.SendData())
func streamToGearman(reader io.Reader, job baseworker.Job) error {
	buffer := make([]byte, 1024)
	for {
		n, err := reader.Read(buffer)
		// Process the data before processing the error (as per the io.Reader docs)
		if n > 0 {
			job.SendData(buffer[:n])
		}
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// sendStderrWarnings sends the last X lines in the stderr output and to the job's warnings
// field
func sendStderrWarnings(buffer io.Reader, job baseworker.Job, warningLines int) error {
	scanner := bufio.NewScanner(buffer)
	// Create a circular buffer for the last X lines
	lastStderrLines := ring.New(warningLines)
	for scanner.Scan() {
		lastStderrLines = lastStderrLines.Next()
		lastStderrLines.Value = scanner.Bytes()
	}
	// Walk forward through the buffer to get all the last X entries. Note that we call next first
	// so that we start at the oldest entry.
	stderrbuf := []byte{}
	for i := 0; i < lastStderrLines.Len(); i++ {
		if lastStderrLines = lastStderrLines.Next(); lastStderrLines.Value != nil {
			stderrbuf = append(stderrbuf, lastStderrLines.Value.([]byte)...)
			stderrbuf = append(stderrbuf, '\n')
		}
	}
	job.SendWarning(stderrbuf)
	return scanner.Err()
}
