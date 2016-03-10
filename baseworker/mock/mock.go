package mock

import "bytes"

// Job is a fake Gearman job for tests
type Job struct {
	Payload, Name, GearmanHandle, ID string
	GearmanErr                       error
	GearmanWarnings                  [][]byte
	DataBuffer                       bytes.Buffer
	Numerator, Denominator           int
}

// CreateMockJob creates an object that implements the gearman-go/worker#Job interface
func CreateMockJob(payload string) *Job {
	return &Job{Payload: payload}
}

// Data returns the Gearman payload
func (m Job) Data() []byte {
	return []byte(m.Payload)
}

// OutData returns the Gearman outpack data
func (m Job) OutData() []byte {
	return m.DataBuffer.Bytes()
}

// Fn returns the job's name
func (m Job) Fn() string {
	return m.Name
}

// Err returns the job's error
func (m Job) Err() error {
	return m.GearmanErr
}

// Handle returns the job's handle
func (m Job) Handle() string {
	return m.GearmanHandle
}

// UniqueId returns the unique id for the job
func (m Job) UniqueId() string {
	return m.ID
}

// Warnings returns the array of jobs warnings
func (m *Job) Warnings() [][]byte {
	return m.GearmanWarnings
}

// SendWarning appends to the array of job warnings
func (m *Job) SendWarning(warning []byte) {
	m.GearmanWarnings = append(m.GearmanWarnings, warning)
}

// SendData appends to the array of job data
func (m *Job) SendData(data []byte) {
	m.DataBuffer.Write(data)
}

// UpdateStatus updates the progress of job
func (m *Job) UpdateStatus(numerator, denominator int) {
	m.Numerator = numerator
	m.Denominator = denominator
}
