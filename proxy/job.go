package proxy

import (
	"encoding/binary"
	"encoding/hex"
)

const (
	nonceOffset = 39
	nonceLength = 4 // bytes
)

// Job is a mining job.  Break it up and send chunks to workers.
type Job struct {
	Blob            string   `json:"blob"`
	ID              string   `json:"job_id"`
	Target          string   `json:"target"`
	SubmittedNonces []string `json:"-"`
}

// NewJobFromServer uses prepares a Job from json data from the pool
func NewJobFromServer(job map[string]interface{}) *Job {
	j := &Job{}
	j.Blob, _ = job["blob"].(string)
	j.ID, _ = job["job_id"].(string)
	j.Target, _ = job["target"].(string)

	return j
}

// NewJob builds a job for distribution to a worker
func NewJob(blobBytes []byte, nonce uint32, id, target string) *Job {
	j := &Job{
		ID:              id,
		Target:          target,
		SubmittedNonces: make([]string, 0),
	}
	nonceBytes := make([]byte, nonceLength, nonceLength)
	binary.BigEndian.PutUint32(nonceBytes, nonce)
	copy(blobBytes[nonceOffset:nonceOffset+nonceLength], nonceBytes)
	j.Blob = hex.EncodeToString(blobBytes)

	return j
}

// Nonce extracts the nonce from the job blob and returns it.
func (j *Job) Nonce() (nonce uint32, blobBytes []byte, err error) {
	blobBytes, err = hex.DecodeString(j.Blob)
	if err != nil {
		return
	}

	nonceBytes := blobBytes[nonceOffset : nonceOffset+nonceLength]
	nonce = binary.BigEndian.Uint32(nonceBytes)

	return
}
