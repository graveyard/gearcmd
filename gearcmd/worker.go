package gearcmd

import (
	"bufio"
	"bytes"
	"container/ring"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/Clever/gearcmd/argsparser"
	"github.com/Clever/gearcmd/baseworker"
	"gopkg.in/Clever/kayvee-go.v3"
	"gopkg.in/Clever/kayvee-go.v3/logger"
)

// TaskConfig defines the configuration for the task.
type TaskConfig struct {
	FunctionName string
	FunctionCmd  string
	WarningLines int
	ParseArgs    bool
	CmdTimeout   time.Duration
	RetryCount   int
	Halt         chan struct{}
}

// Process runs the Gearman job by running the configured task.
// We need to implement the Task interface so we return (byte[], error)
// though the byte[] is always nil.
func (conf TaskConfig) Process(job baseworker.Job) (b []byte, returnErr error) {
	jobData := string(job.Data())
	jobID := baseworker.GetJobID(job)
	if jobID == "" {
		jobID = strconv.Itoa(rand.Int())
		fmt.Println(kayvee.FormatLog("gearcmd", kayvee.Info, "rand-job-id",
			logger.M{"msg": "no job id parsed, random assigned."}))
	}

	// NOTE: we create our logger(s) inline to ensure that they include valuable metadata such as
	// 'job_id' and 'function' while still avoiding mutating the global state of the logger(s).
	context := logger.M{"job_id": jobID, "function": conf.FunctionName}
	lg := logger.NewWithContext("gearcmd", context)
	// legacy logger used to maintain existing alerts.
	// Once all workers are migrated to using gearcmd >= v0.5.0 and the alarms are switched over,
	// then we can remove this logger
	legacyLg := logger.NewWithContext("gearman", context)

	// This wraps the actual processing to do some logging
	lg.InfoD("START", logger.M{"job_data": jobData})
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

		err = conf.doProcess(job, extraEnvVars, try, lg)
		end := time.Now()

		// Return if the job was successful.
		if err == nil {
			lg.InfoD("SUCCESS", logger.M{
				"type": "counter"})
			legacyLg.InfoD("success", logger.M{
				"type": "counter"})
			// NOTE: we roll our own guage function because we want to include the "success" key
			lg.InfoD("END", logger.M{
				"type":    "gauge",
				"value":   1,
				"success": true})
			// Hopefully none of our jobs last long enough for a uint64...
			// Note that we cannot use lg.GaugeIntD because duration is uint64
			lg.InfoD("duration", logger.M{
				"value":    uint64(end.Sub(start).Seconds() * 1000),
				"type":     "gauge",
				"job_data": jobData})
			return nil, nil
		}

		returnErr = err
		if try != conf.RetryCount {
			lg.ErrorD("RETRY", logger.M{
				"value":    0,
				"success":  false,
				"error":    err.Error(),
				"job_data": jobData})
		}
	}

	lg.InfoD("FAILURE", logger.M{"type": "counter"})
	legacyLg.InfoD("failure", logger.M{"type": "counter"})
	lg.ErrorD("END", logger.M{"job_data": jobData})
	return nil, returnErr
}

func (conf TaskConfig) doProcess(job baseworker.Job, envVars []string, tryCount int, lg *logger.Logger) error {
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

	// No timeout
	if conf.CmdTimeout == 0 {
		select {
		case err := <-done:
			// Will be nil if the channel was closed without any errors
			return err
		case <-conf.Halt:
			if err := sigtermProcess(cmd.Process, lg); err != nil {
				return fmt.Errorf("error sending SIGTERM to process: %s", err)
			}
			return fmt.Errorf("killed process due to sigterm")
		}
	}
	select {
	case err := <-done:
		// Will be nil if the channel was closed without any errors
		return err
	case <-conf.Halt:
		if err := sigtermProcess(cmd.Process, lg); err != nil {
			return fmt.Errorf("error sending SIGTERM to process: %s", err)
		}
		return nil
	case <-time.After(conf.CmdTimeout):
		if err := sigtermProcess(cmd.Process, lg); err != nil {
			return fmt.Errorf("error timing out process after %s: %s", conf.CmdTimeout.String(), err)
		}
		return fmt.Errorf("process timed out after %s", conf.CmdTimeout.String())
	}
}

func sigtermProcess(p *os.Process, lg *logger.Logger) error {
	// kill entire group of process spawned by our cmd.Process
	pgid, err := syscall.Getpgid(p.Pid)
	if err != nil {
		return fmt.Errorf("unable to get pgid, error: %s", err)
	}
	lg.InfoD("killing-pgid", logger.M{"pgid": pgid})
	// minus sign required to kill PGIDs
	// we use SIGTERM so that the subprocess can gracefully exit
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("unable to kill process, error: %s", err)
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
	for i := 0; i < lastStderrLines.Len(); i++ {
		if lastStderrLines = lastStderrLines.Next(); lastStderrLines.Value != nil {
			job.SendWarning(append(lastStderrLines.Value.([]byte), byte('\n')))
		}
	}
	return scanner.Err()
}
