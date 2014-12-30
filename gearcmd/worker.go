package gearcmd

import (
	"bufio"
	"bytes"
	"container/ring"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Clever/baseworker-go"
	"github.com/Clever/gearcmd/argsparser"
	kayvee "github.com/Clever/kayvee-go"
)

// TaskConfig defines the configuration for the task.
type TaskConfig struct {
	FunctionName, FunctionCmd string
	WarningLines              int
	ParseArgs                 bool
	CmdTimeout                time.Duration
}

// Process runs the Gearman job by running the configured task.
// We need to implement the Task interface so we return (byte[], error)
// though the byte[] is always nil.
func (conf TaskConfig) Process(job baseworker.Job) ([]byte, error) {
	// This wraps the actual processing to do some logging
	log.Printf(kayvee.FormatLog("gearcmd", "info", "START",
		map[string]interface{}{"function_name": conf.FunctionName, "job_id": getJobId(job), "job_data": string(job.Data())}))
	start := time.Now()
	err := conf.doProcess(job)
	end := time.Now()
	if err != nil {
		log.Printf(kayvee.FormatLog("gearcmd", "error", "END_WITH_ERROR",
			map[string]interface{}{"function_name": conf.FunctionName, "job_id": getJobId(job),
				"error_message": err.Error(), "job_data": string(job.Data())}))
	} else {
		durationStr := fmt.Sprintf("%d", int32(end.Sub(start).Seconds() * 1000))
		log.Printf(kayvee.FormatLog("gearcmd", "info", "END",
			map[string]interface{}{"function_name": conf.FunctionName, "job_id": getJobId(job),
				"job_data": string(job.Data()), "duration": durationStr}))
	}
	return nil, err
}

// getJobId returns the jobId from the job handle
func getJobId(job baseworker.Job) string {
	splits := strings.Split(job.Handle(), ":")
	return splits[len(splits)-1]
}

func (conf TaskConfig) doProcess(job baseworker.Job) error {

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
			return err
		}
	} else {
		args = []string{string(job.Data())}

	}
	cmd := exec.Command(conf.FunctionCmd, args...)

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
			job.SendWarning(lastStderrLines.Value.([]byte))
		}
	}
	return scanner.Err()
}
