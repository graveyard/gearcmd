package gearcmd

import (
	"bufio"
	"bytes"
	"container/ring"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/Clever/baseworker-go"
	"github.com/Clever/gearcmd/argsparser"
)

// TaskConfig defines the configuration for the task.
type TaskConfig struct {
	FunctionName, FunctionCmd string
	WarningLines              int
}

// Process runs the Gearman job by running the configured task.
func (conf TaskConfig) Process(job baseworker.Job) ([]byte, error) {
	// This wraps the actual processing to do some logging
	log.Printf("STARTING %s %s %s", conf.FunctionName, job.UniqueId(), string(job.Data()))
	result, err := conf.doProcess(job)
	if err != nil {
		log.Printf("ENDING %s %s %s", conf.FunctionName, job.UniqueId(), err.Error())
	} else {
		log.Printf("ENDING %s %s", conf.FunctionName, job.UniqueId())
	}
	return result, err
}

func (conf TaskConfig) doProcess(job baseworker.Job) ([]byte, error) {

	defer func() {
		// If we panicked then set the panic message as a warning. Gearman-go will
		// handle marking this job as failed.
		if r := recover(); r != nil {
			err := r.(error)
			job.SendWarning([]byte(err.Error()))
		}
	}()
	args, err := argsparser.ParseArgs(string(job.Data()))
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(conf.FunctionCmd, args...)

	// Write the stdout and stderr of the process to both this process' stdout and stderr
	// and also write it to a byte buffer so that we can return it with the Gearman job
	// data as necessary.
	var stderrbuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrbuf)
	stdoutReader, stdoutWriter := io.Pipe()
	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutWriter)
	finishedProcessingStdout := make(chan error)
	go func() {
		finishedProcessingStdout <- streamToGearman(stdoutReader, job)
	}()
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	stdoutWriter.Close()
	sendStderrWarnings(&stderrbuf, job, conf.WarningLines)
	if err = <-finishedProcessingStdout; err != nil {
		return nil, err
	}
	return nil, nil
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
		if err != nil && err != io.EOF {
			return err
		} else if err != nil {
			return nil
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
