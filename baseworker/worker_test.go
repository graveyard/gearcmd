package baseworker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Clever/baseworker-go/mock"
)

// GetTestSigtermHandler return an no-op sigterm handler for the tests so that they
// don't call exit(0) which is the default behavior.
func getTestSigtermHandler() SigtermHandler {
	return func(worker *Worker) {}
}

// TestJobFuncConversion tests that our JobFunc is called when 'worker.fn' is called with a job.
func TestJobFuncConversion(t *testing.T) {
	payload := "I'm a payload!"
	jobFunc := func(job Job) ([]byte, error) {
		if string(job.Data()) != payload {
			t.Fatalf("expected payload %s, received %s", payload, string(job.Data()))
		}
		return []byte{}, nil
	}
	worker := NewWorker("test", jobFunc)
	worker.sigtermHandler = getTestSigtermHandler()
	worker.fn(mock.CreateMockJob(payload))
}

func makeTCPServer(addr string, handler func(conn net.Conn) error) (net.Listener, chan error) {
	channel := make(chan error)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		if err := handler(conn); err != nil {
			channel <- err
		}
	}()

	return listener, channel
}

func readBytes(reader io.Reader, size uint32) ([]byte, error) {
	buf := make([]byte, size)
	_, err := io.ReadFull(reader, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func fromBigEndianBytes(buf []byte) (uint32, error) {
	var num uint32
	if err := binary.Read(bytes.NewReader(buf), binary.BigEndian, &num); err != nil {
		return 0, err
	}
	return num, nil
}

func toBigEndianBytes(num uint32) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, num); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readGearmanHeader(reader io.Reader) (uint32, uint32, error) {
	header, err := readBytes(reader, 12)
	if err != nil {
		return 0, 0, err
	}
	cmd, err := fromBigEndianBytes(header[4:8])
	if err != nil {
		return 0, 0, err
	}
	cmdLen, err := fromBigEndianBytes(header[8:12])
	if err != nil {
		return 0, 0, err
	}
	return cmd, cmdLen, nil
}

func readGearmanCommand(reader io.Reader) (uint32, string, error) {
	cmd, dataSize, err := readGearmanHeader(reader)
	if err != nil {
		return 0, "", err
	}
	body, err := readBytes(reader, dataSize)
	if err != nil {
		return 0, "", err
	}
	return cmd, string(body), nil
}

// MakeJobAssignServer creates a server that responds to connection with a JobAssign message
// with the specified name and workload.
func makeJobAssignServer(addr, name, workload string) (net.Listener, chan error) {
	return makeTCPServer(addr, func(conn net.Conn) error {
		handle := "job_handle"
		function := name
		body := []byte(handle + string('\x00') + function + string('\x00') + workload)

		response, err := makeGearmanCommand(11, body)
		if err != nil {
			return err
		}
		if _, err := conn.Write(response); err != nil {
			return err
		}
		return nil
	})
}

// TestCanDo tests that Listen properly sends a 'CAN_DO worker_name' packet to the TCP server.
func TestCanDo(t *testing.T) {

	var channel chan error
	var listener net.Listener

	name := "worker_name"

	listener, channel = makeTCPServer(":1337", func(conn net.Conn) error {
		cmd, body, err := readGearmanCommand(conn)
		if err != nil {
			return err
		}
		// 1 = CAN_DO
		if cmd != 1 {
			return fmt.Errorf("expected command 1 (CAN_DO), received command %d", cmd)
		}
		if body != "worker_name" {
			return fmt.Errorf("expected '%s', received '%s'", name, body)
		}
		close(channel)
		return nil
	})
	defer listener.Close()

	worker := NewWorker(name, func(job Job) ([]byte, error) {
		return []byte{}, nil
	})
	worker.sigtermHandler = getTestSigtermHandler()
	go worker.Listen("localhost", "1337")

	for err := range channel {
		t.Fatal(err)
	}
	worker.w.Shutdown()
}

func makeGearmanCommand(cmd uint32, body []byte) ([]byte, error) {
	header := []byte{'\x00', 'R', 'E', 'S'}
	// 11 is JOB_ASSIGN
	cmdBytes, err := toBigEndianBytes(cmd)
	if err != nil {
		return nil, err
	}
	header = append(header, cmdBytes...)
	bodySize, err := toBigEndianBytes(uint32(len(body)))
	if err != nil {
		return nil, err
	}
	header = append(header, bodySize...)
	response := append(header, body...)
	return response, nil
}

// TestJobAssign tests that the worker runs the JOB_FUNC if the server sends a 'JOB_ASSIGN' packet.
func TestJobAssign(t *testing.T) {

	name := "worker_name"
	workload := "the_workload"

	var channel chan error
	var listener net.Listener

	listener, channel = makeJobAssignServer(":1337", name, workload)
	defer listener.Close()

	worker := NewWorker(name, func(job Job) ([]byte, error) {
		if string(job.Data()) != workload {
			close(channel)
			t.Fatalf("expected workload of '%s', received '%s'", workload, string(job.Data()))
		}
		close(channel)
		return []byte{}, nil
	})
	worker.sigtermHandler = getTestSigtermHandler()
	go worker.Listen("localhost", "1337")

	for err := range channel {
		t.Fatal(err)
	}
	worker.w.Shutdown()
}

func TestShutdownWaitsForJobCompletion(t *testing.T) {
	var wg sync.WaitGroup
	name := "shutdown_worker"
	var listener net.Listener

	listener, _ = makeJobAssignServer(":1337", name, "")
	defer listener.Close()

	ranJob := false
	worker := NewWorker(name, func(job Job) ([]byte, error) {
		wg.Done()
		time.Sleep(time.Duration(10 * time.Millisecond))
		ranJob = true
		return []byte{}, nil
	})
	worker.sigtermHandler = getTestSigtermHandler()

	wg.Add(1)
	go worker.Listen("localhost", "1337")
	wg.Wait()
	worker.w.Shutdown()
	if !ranJob {
		t.Error("Didn't run job")
	}
}

func TestHandleSignal(t *testing.T) {
	worker := NewWorker("SignalWorker", func(job Job) ([]byte, error) {
		return nil, nil
	})
	ranSigtermHandler := false
	worker.sigtermHandler = func(worker *Worker) {
		ranSigtermHandler = true
	}
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	time.Sleep(time.Duration(10 * time.Millisecond))
	if !ranSigtermHandler {
		t.Error("Didn't run sigterm handler")
	}
}
