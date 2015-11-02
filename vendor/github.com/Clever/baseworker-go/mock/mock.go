package mock

import "bytes"

// MockJob is a fake Gearman job for tests
type MockJob struct {
	Payload, Name, GearmanHandle, Id string
	GearmanErr                       error
	GearmanWarnings                  [][]byte
	DataBuffer                       bytes.Buffer
	Numerator, Denominator           int
}

// CreateMockJob creates an object that implements the gearman-go/worker#Job interface
func CreateMockJob(payload string) *MockJob {
	return &MockJob{Payload: payload}
}

// Data returns the Gearman payload
func (m MockJob) Data() []byte {
	return []byte(m.Payload)
}

// OutData returns the Gearman outpack data
func (m MockJob) OutData() []byte {
	return m.DataBuffer.Bytes()
}

// Fn returns the job's name
func (m MockJob) Fn() string {
	return m.Name
}

// Err returns the job's error
func (m MockJob) Err() error {
	return m.GearmanErr
}

// Handle returns the job's handle
func (m MockJob) Handle() string {
	return m.GearmanHandle
}

// UniqueId returns the unique id for the job
func (m MockJob) UniqueId() string {
	return m.Id
}

// Warnings returns the array of jobs warnings
func (m *MockJob) Warnings() [][]byte {
	return m.GearmanWarnings
}

// SendWarning appends to the array of job warnings
func (m *MockJob) SendWarning(warning []byte) {
	m.GearmanWarnings = append(m.GearmanWarnings, warning)
}

// SendData appends to the array of job data
func (m *MockJob) SendData(data []byte) {
	m.DataBuffer.Write(data)
}

// UpdateStatus updates the progress of job
func (m *MockJob) UpdateStatus(numerator, denominator int) {
	m.Numerator = numerator
	m.Denominator = denominator
}
