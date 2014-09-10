package main

import (
	mock "github.com/Clever/baseworker-go/mock"
	"github.com/Clever/gearcmd/gearcmd"
)

// Simple program to run the worker Process method. We have this so that we can easily test forwarding
// stderr from the worker process to this process. If we don't separate this into a another process then
// we mix the stderr of the test process itself with the worker process which makes things a bit trickier.
func main() {
	mockJob := mock.CreateMockJob("IgnorePayload")
	config := gearcmd.TaskConfig{FunctionName: "name", FunctionCmd: "testscripts/logStderr.sh", WarningLines: 5}
	config.Process(mockJob)
}
