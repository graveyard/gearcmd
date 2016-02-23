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
	"strings"
	"syscall"
	"time"

	"github.com/Clever/gearcmd/argsparser"
	"github.com/Clever/gearcmd/baseworker"
	"gopkg.in/Clever/kayvee-go.v2/logger"
)

// TaskConfig defines the configuration for the task.
type TaskConfig struct {
	FunctionName string
	FunctionCmd  string
	WarningLines int
	ParseArgs    bool
	CmdTimeout   time.Duration
	RetryCount   int
}

var (
	lg = logger.New("gearcmd")
)

// Process runs the Gearman job by running the configured task.
// We need to implement the Task interface so we return (byte[], error)
// though the byte[] is always nil.
func (conf TaskConfig) Process(job baseworker.Job) ([]byte, error) {
	jobID := getJobID(job)
	if jobID == "" {
		jobID = strconv.Itoa(rand.Int())
		lg.InfoD("rand-job-id", logger.M{"msg": "no job id parsed, random assigned."})
	}

	// We create a temporary directory to be used as the work directory of the process. Currently
	// we do not handle the case of this being used multiple times when retrying, files are
	// expected to be overwritten.
	tempDirPath, err := ioutil.TempDir("/tmp", fmt.Sprintf("%s-%s", conf.FunctionName, jobID))
	if err != nil {
		lg.CriticalD("tempdir-failure", logger.M{"error": err.Error()})
		return nil, err
	}
	defer os.RemoveAll(tempDirPath)

	// insert the job id and the work directory path into the environment
	extraEnvVars := []string{
		fmt.Sprintf("JOB_ID=%s", jobID),
		fmt.Sprintf("WORK_DIR=%s", tempDirPath),
	}

	// This wraps the actual processing to do some logging
	lg.InfoD("START", logger.M{
		"function": conf.FunctionName,
		"job_id":   jobID,
		"job_data": string(job.Data())})

	start := time.Now()
	for {
		err := conf.doProcess(job, extraEnvVars)
		end := time.Now()
		data := logger.M{
			"function": conf.FunctionName,
			"job_id":   jobID,
			"job_data": string(job.Data()),
			"type":     "gauge",
		}

		// Return if the job was successful.
		if err == nil {
			lg.InfoD("success", logger.M{
				"type":     "counter",
				"function": conf.FunctionName})
			data["value"] = 1
			data["success"] = true
			lg.InfoD("END", data)
			// Hopefully none of our jobs last long enough for a uint64...
			lg.InfoD("duration", logger.M{
				"value":    uint64(end.Sub(start).Seconds() * 1000),
				"type":     "gauge",
				"function": conf.FunctionName})
			return nil, nil
		}
		data["error_message"] = err.Error()
		data["value"] = 0
		data["success"] = false
		// Return if the job has no more retries.
		if conf.RetryCount <= 0 {
			lg.InfoD("failure", logger.M{
				"type":     "counter",
				"function": conf.FunctionName})
			lg.ErrorD("END", data)
			return nil, err
		}
		conf.RetryCount--
		lg.ErrorD("RETRY", data)
	}
}

// getJobID returns the jobId from the job handle
func getJobID(job baseworker.Job) string {
	splits := strings.Split(job.Handle(), ":")
	return splits[len(splits)-1]
}

func (conf TaskConfig) doProcess(job baseworker.Job, envVars []string) error {
	defer func() {
		// If we panicked then set the panic message as a warning. Gearman-go will
		// handle marking this job as failed.
		if r := recover(); r != nil {
			err := r.(error)
			job.SendWarning([]byte(err.Error()))
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
	go func() {
		defer close(done)

		finishedProcessingStdout := make(chan error)
		go func() {
			finishedProcessingStdout <- streamToGearman(stdoutReader, job)
		}()

		// Save the cmdErr. We want to process stdout and stderr before we return it
		cmdErr := cmd.Run()
		stdoutWriter.Close()

		stdoutErr := <-finishedProcessingStdout
		if cmdErr != nil {
			done <- cmdErr
		} else if stdoutErr != nil {
			done <- stdoutErr
		}
	}()
	// No timeout
	if conf.CmdTimeout == 0 {
		// Will be nil if the channel was closed without any errors
		return <-done
	}
	select {
	case err := <-done:
		// Will be nil if the channel was closed without any errors
		return err
	case <-time.After(conf.CmdTimeout):
		// kill entire group of process spawned by our cmd.Process
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		lg.InfoD("killing-pgid", logger.M{"pgid": pgid})
		if err != nil {
			return fmt.Errorf("process timeout after %s. Unable to get pgid, error: %s", conf.CmdTimeout.String(), err.Error())
		}
		// minus sign required to kill PGIDs
		// we use SIGTERM so that the subprocess can gracefully exit
		err = syscall.Kill(-pgid, syscall.SIGTERM)
		if err != nil {
			return fmt.Errorf("process timeout after %s. Unable to kill process, error: %s", conf.CmdTimeout.String(), err.Error())
		}
		return fmt.Errorf("process timed out after %s", conf.CmdTimeout.String())
	}
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
