package gearcmd

import (
	"bytes"
	"container/ring"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	mock "github.com/Clever/gearcmd/baseworker/mock"
	gearcmdconfig "github.com/Clever/gearcmd/config"
	"github.com/facebookgo/clock"
	"github.com/stretchr/testify/assert"
)

// Helper function to get the response for a job that should be successful
func getSuccessResponse(payload string, cmd string, t *testing.T) string {
	config := TaskConfig{FunctionName: "name", FunctionCmd: cmd, WarningLines: 5, ParseArgs: true}
	return getSuccessResponseWithConfig(payload, config, t)
}

func getSuccessResponseWithConfig(payload string, config TaskConfig, t *testing.T) string {
	mockJob := mock.CreateMockJob(payload)
	mockJob.GearmanHandle = "H:lap:123"
	_, err := config.Process(mockJob)
	assert.Nil(t, err)
	return string(mockJob.OutData())
}

func testRetryOnFailure(retryCount int) ([]byte, error) {
	// Get a temp file for the script to hold its state.
	file, err := ioutil.TempFile("", "temp")
	if err != nil {
		return nil, fmt.Errorf("Could not create temporary file: %s", err.Error())
	}
	filename := file.Name()
	defer os.Remove(filename)
	defer file.Close()
	mockJob := mock.CreateMockJob(filename)
	config := TaskConfig{
		FunctionName: "name",
		FunctionCmd:  "testscripts/succeedOnFifthRun.sh",
		RetryCount:   retryCount,
	}
	return config.Process(mockJob)
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

func TestRetryOnFailureScriptEventuallySucceeds(t *testing.T) {
	response, err := testRetryOnFailure(4)
	assert.Nil(t, response)
	assert.NoError(t, err)
}

func TestRetryOnFailureScriptFails(t *testing.T) {
	response, err := testRetryOnFailure(3)
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
	assert.Equal(t, 1, len(warnings))
	assert.Equal(t, string(warnings[0]), "stderr7\nstderr8\n")
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
	assert.Equal(t, 1, len(warnings))
	assert.Equal(t, string(warnings[0]), "stderr7\nstderr8\n")
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
	lastWarning := warnings[0] // there's only 1 warning
	assert.Equal(t, "stderr1\nstderr2\n", string(lastWarning))
	assert.Equal(t, "stdout1\nstdout2\n", string(mockJob.OutData()))
}

func TestStderrCapturedWarningsOnFailedJobs(t *testing.T) {
	mockJob := mock.CreateMockJob("IgnorePayload")
	config := TaskConfig{FunctionName: "name", FunctionCmd: "testscripts/logStderrFail.sh", WarningLines: 2, ParseArgs: true}
	_, err := config.Process(mockJob)
	assert.Error(t, err)
	warnings := mockJob.Warnings()
	assert.Equal(t, warnings, [][]byte{[]byte("stderr7\nstderr8\n")})
}

func TestMockJobName(t *testing.T) {
	mockJob := &mock.Job{GearmanHandle: "H:lap:123"}
	assert.Equal(t, "123", getJobID(mockJob))

	mockJob = &mock.Job{GearmanHandle: ""}
	assert.Equal(t, "", getJobID(mockJob))
}

func TestRemoveQuotesIfParseArgs(t *testing.T) {
	response := getSuccessResponse(`{"key":"value"}`, "testscripts/echoInput.sh", t)
	assert.Equal(t, "{key:value}\n\n", response)
}

func TestNoParse(t *testing.T) {
	config := TaskConfig{FunctionName: "name", FunctionCmd: "testscripts/echoInput.sh",
		WarningLines: 5, ParseArgs: false}
	response := getSuccessResponseWithConfig(`{"key":"value"}`, config, t)
	assert.Equal(t, "{\"key\":\"value\"}\n\n", response)
}

func TestSendStderrWarnings(t *testing.T) {
	stdErrStr := ""
	for i := 0; i < 30; i++ {
		stdErrStr += fmt.Sprintf("line #%d\n", i)
	}
	expectedStr := ""
	for i := 20; i < 30; i++ {
		expectedStr += fmt.Sprintf("line #%d\n", i)
	}
	mockJob := mock.CreateMockJob("")

	assert.Nil(t, sendStderrWarnings(bytes.NewBufferString(stdErrStr), mockJob, 10))
	assert.Equal(t, expectedStr, string(bytes.Join(mockJob.GearmanWarnings, []byte{})))
}

func TestEnvJobIDInsertion(t *testing.T) {
	response := getSuccessResponse("", "testscripts/output_env.sh", t)
	assert.Contains(t, strings.Split(response, "\n"), "JOB_ID=123")
	// there will be other characters appended to the WORK_DIR so we look for a simple substring match
	assert.Contains(t, response, "WORK_DIR=/tmp/name-123-0")
}

func TestHaltGraceful(t *testing.T) {
	mockJob := mock.CreateMockJob("IgnorePayload")
	haltChan := make(chan struct{})
	go func() {
		time.Sleep(1 * time.Second)
		close(haltChan)
	}()
	config := TaskConfig{
		FunctionName: "name",
		FunctionCmd:  "testscripts/stderrAndHang.sh",
		WarningLines: 2,
		ParseArgs:    true,
		CmdTimeout:   2 * time.Second,
		Halt:         haltChan,
	}
	_, err := config.Process(mockJob)
	assert.NoError(t, err)
}

func TestProcessWithErrorBackoff(t *testing.T) {
	// Get a temp file for the script to hold its state.
	file, err := ioutil.TempFile("", "temp")
	assert.NoError(t, err)
	filename := file.Name()
	defer os.Remove(filename)
	defer file.Close()
	mockJob := mock.CreateMockJob(filename)
	config := TaskConfig{
		FunctionName:            "name",
		FunctionCmd:             "testscripts/nonZeroExit.sh",
		LastResults:             ring.New(2),
		ErrorResultsBackoffRate: 10 * time.Millisecond,
	}
	mockClock := clock.NewMock()
	gearcmdconfig.Clock = mockClock
	defer func() {
		gearcmdconfig.Clock = clock.New()
	}()

	assert.Equal(t, config.errorResultsBackoff, 0)
	response, err := config.ProcessWithErrorBackoff(mockJob)
	assert.Nil(t, response)
	assert.EqualError(t, err, "exit status 2")
	assert.Equal(t, config.errorResultsBackoff, config.ErrorResultsBackoffRate)

	done := make(chan bool)
	go func() {
		response, err := config.ProcessWithErrorBackoff(mockJob)
		assert.Nil(t, response)
		assert.EqualError(t, err, "exit status 2")
		done <- true
	}()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, config.errorResultsBackoff, config.ErrorResultsBackoffRate)
	mockClock.Add(config.ErrorResultsBackoffRate)
	<-done
	assert.Equal(t, config.errorResultsBackoff, config.ErrorResultsBackoffRate*2)

}
