package proxy

import (
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/trey-jones/xmrwasp/config"
	"github.com/trey-jones/xmrwasp/logger"
)

const (
	// ValidateNormal just checks that there is a valid job ID and the share is
	// not a duplicate for this job
	ValidateNormal = iota

	// ValidateExtra checks that the result difficulty meets the target
	ValidateExtra

	// TODO ValidateFull checks nonce against blob for result
	// maybe not worth it!
)

const (
	// need more information about this uint64
	shareValueOffset = 24
	shareValueLength = 8
)

var (
	ErrMalformedShareResult = errors.New("result is the correct length")
	ErrDiffTooLow           = errors.New("share difficulty too low")
)

type share struct {
	AuthID string `json:"id"`
	JobID  string `json:"job_id"`
	Nonce  string `json:"nonce"`
	Result string `json:"result"`

	Error    chan error        `json:"-"`
	Response chan *StatusReply `json:"-"`
}

// might return an invalid share, and that's fine - will fail validation
func newShare(params map[string]interface{}) *share {
	s := &share{
		Error:    make(chan error, 1),
		Response: make(chan *StatusReply, 1),
	}

	if jobID, ok := params["job_id"]; ok {
		s.JobID = jobID.(string)
	}

	if nonce, ok := params["nonce"]; ok {
		s.Nonce = nonce.(string)
	}

	if result, ok := params["result"]; ok {
		s.Result = result.(string)
	}

	return s
}

func (s *share) validate(j *Job) error {
	// normal validate for no duplicate
	for _, n := range j.SubmittedNonces {
		if n == s.Nonce {
			return ErrDuplicateShare
		}
	}
	validateLevel := config.Get().ShareValidation
	if validateLevel > ValidateNormal {
		// second level validation - make sure diff is high enough
		err := s.validateDifficulty(j)
		if err != nil {
			return err
		}
	}

	if validateLevel > ValidateExtra {
		return s.validateResult(j)
	}

	return nil
}

func (s *share) validateDifficulty(j *Job) error {
	target, err := j.getTargetUint64()
	if err != nil {
		// don't try to validate, just record so we can fix later
		logger.Get().Println("error validating difficulty: ", err)
		return nil
	}

	result, err := s.getResultUint64()
	if err != nil {
		logger.Get().Println("error validating difficulty: ", err)
		return err
	}

	if result < target {
		return ErrDiffTooLow
	}

	return nil
}

// not implemented, and no rush to do so
func (s *share) validateResult(j *Job) error {
	return nil
}

func (s *share) getResultUint64() (uint64, error) {
	resultBytes, err := hex.DecodeString(s.Result)
	if err != nil {
		return 0, err
	}

	if len(resultBytes) < shareValueOffset+shareValueLength {
		return 0, ErrMalformedShareResult
	}

	valueBytes := resultBytes[shareValueOffset : shareValueOffset+shareValueLength]

	return binary.BigEndian.Uint64(valueBytes), nil
}
