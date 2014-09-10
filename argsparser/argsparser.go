package argsparser

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

// ParseArgs converts the command line specified into a slice of the command line arguments.
func ParseArgs(commandline string) ([]string, error) {
	// This is a bit hacky, but we couldn't think of a better way to do it.
	// We create a bash script and in that file we run a bash command that parses the
	// command line arguments we wrote to the file. The bash script outputs each of the
	// parsed arguments to stdout, separated by \n. We parse the stdout and return
	// that to the caller.
	filename, err := createAndWriteFile(commandline)
	if err != nil {
		return nil, err
	}
	defer os.Remove(filename)
	cmd := exec.Command(filename)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	response, err := ioutil.ReadAll(stdout)
	if err != nil {
		return nil, err
	}
	if err = cmd.Wait(); err != nil {
		return nil, err
	}
	argsArray := strings.Split(string(response), "\n")
	// Remove the last element of the argsArray because the output ends with an endline
	// and has an empty last element
	argsArray = argsArray[0 : len(argsArray)-1]
	return argsArray, nil
}

// createAndWriteFile creates the temporary file, writes the bash command to it and returns
// the filepath
func createAndWriteFile(commandline string) (string, error) {
	file, err := ioutil.TempFile("/tmp", "parseArgs")
	if err != nil {
		return "", err
	}
	defer file.Close()
	file.WriteString("#!/bin/bash\n")
	file.WriteString("bash -c 'while test ${#} -gt 0; do echo $1; shift; done;' _ " + commandline + "\n")

	if err := file.Chmod(0744); err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}
