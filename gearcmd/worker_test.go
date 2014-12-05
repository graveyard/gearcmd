package gearcmd

import (
	"io/ioutil"
	"os/exec"
	"strings"
	"testing"
	"time"

	mock "github.com/Clever/baseworker-go/mock"
	"github.com/stretchr/testify/assert"
)

// Helper function to get the response for a job that should be successful
func getSuccessResponse(payload string, cmd string, t *testing.T) string {
	config := TaskConfig{FunctionName: "name", FunctionCmd: cmd, WarningLines: 5, ParseArgs: true}
	return getSuccessResponseWithConfig(payload, config, t)
}

func getSuccessResponseWithConfig(payload string, config TaskConfig, t *testing.T) string {
	mockJob := mock.CreateMockJob(payload)
	_, err := config.Process(mockJob)
	assert.Nil(t, err)
	return string(mockJob.OutData())
}

func TestSuccessResponse(t *testing.T) {
	response := getSuccessResponse("IgnorePayload", "testscripts/success.sh", t)
	assert.Equal(t, "SuccessResponse\n", response)
}

func TestErrorOnNonZeroExitCode(t *testing.T) {
	mockJob := mock.CreateMockJob("IgnorePayload")
	config := TaskConfig{FunctionName: "name", FunctionCmd: "testscripts/nonZeroExit.sh", WarningLines: 5}
	response, err := config.Process(mockJob)
	assert.Nil(t, response)
	assert.EqualError(t, err, "exit status 2")
}

func TestWorkerRecievesInputData(t *testing.T) {
	response := getSuccessResponse("arg1 arg2", "testscripts/echoInput.sh", t)
	assert.Equal(t, "arg1\narg2\n", response)
}

func TestStderrForwardedToProcess(t *testing.T) {
	// This test creates a child process because we want to make sure that the stderr of the worker
	// process is forwarded to the child process correctly. If we don't create a child process we
	// end up checking our own process' stderr which is a pain.
	cmd := exec.Command("go", "run", "testscripts/test_stderr.go")
	stderr, err := cmd.StderrPipe()
	assert.NoError(t, err)
	assert.NoError(t, cmd.Start())
	response, err := ioutil.ReadAll(stderr)

	assert.NoError(t, err)
	assert.NoError(t, cmd.Wait())
	if !strings.Contains(string(response), "stderr") {
		t.Fatal("Missing expected stderr output: " + string(response))
	}
}

func TestStderrCapturedInWarnings(t *testing.T) {
	mockJob := mock.CreateMockJob("IgnorePayload")
	config := TaskConfig{FunctionName: "name", FunctionCmd: "testscripts/logStderr.sh", WarningLines: 2, ParseArgs: true}
	_, err := config.Process(mockJob)
	assert.NoError(t, err)
	warnings := mockJob.Warnings()
	assert.Equal(t, 2, len(warnings))
	assert.Equal(t, string(warnings[0]), "stderr7")
	assert.Equal(t, string(warnings[1]), "stderr8")
}

func TestStderrCapturedWhenHanging(t *testing.T) {
	mockJob := mock.CreateMockJob("IgnorePayload")
	config := TaskConfig{
		FunctionName: "name",
		FunctionCmd:  "testscripts/stderrAndHang.sh",
		WarningLines: 2,
		ParseArgs:    true,
		CmdTimeout:   time.Second,
	}
	_, err := config.Process(mockJob)
	assert.EqualError(t, err, "process timed out after 1s")
	warnings := mockJob.Warnings()
	assert.Equal(t, 2, len(warnings))
	assert.Equal(t, string(warnings[0]), "stderr7")
	assert.Equal(t, string(warnings[1]), "stderr8")
}

func TestHandleStderrAndStdoutTogether(t *testing.T) {
	mockJob := mock.CreateMockJob("IgnorePayload")
	config := TaskConfig{FunctionName: "name", FunctionCmd: "testscripts/logStdoutAndStderr.sh", WarningLines: 5, ParseArgs: true}
	_, err := config.Process(mockJob)
	assert.NoError(t, err)
	warnings := mockJob.Warnings()
	if len(warnings) == 0 {
		t.Fatal("Empty warnings")
	}
	lastWarning := warnings[len(warnings)-1]
	assert.Equal(t, "stderr2", string(lastWarning))
	assert.Equal(t, "stdout1\nstdout2\n", string(mockJob.OutData()))
}

func TestStderrCapturedWarningsOnFailedJobs(t *testing.T) {
	mockJob := mock.CreateMockJob("IgnorePayload")
	config := TaskConfig{FunctionName: "name", FunctionCmd: "testscripts/logStderrFail.sh", WarningLines: 2, ParseArgs: true}
	_, err := config.Process(mockJob)
	assert.Error(t, err)
	warnings := mockJob.Warnings()
	assert.Equal(t, warnings, [][]byte{[]byte("stderr7"), []byte("stderr8")})
}

func TestMockJobName(t *testing.T) {
	mockJob := &mock.MockJob{GearmanHandle: "H:lap:123"}
	assert.Equal(t, "123", getJobId(mockJob))

	mockJob = &mock.MockJob{GearmanHandle: ""}
	assert.Equal(t, "", getJobId(mockJob))
}

func TestRemoveQuotesIfParseArgs(t *testing.T) {
	response := getSuccessResponse("{\"key\":\"value\"}", "testscripts/echoInput.sh", t)
	assert.Equal(t, "{key:value}\n\n", response)
}

func TestNoParse(t *testing.T) {
	config := TaskConfig{FunctionName: "name", FunctionCmd: "testscripts/echoInput.sh",
		WarningLines: 5, ParseArgs: false}
	response := getSuccessResponseWithConfig("{\"key\":\"value\"}", config, t)
	assert.Equal(t, "{\"key\":\"value\"}\n\n", response)
}
