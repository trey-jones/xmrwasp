package stratum

type Request struct {
	ID     uint64                 `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

type Response struct {
	ID     uint64                 `json:"id"`
	Result map[string]interface{} `json:"result"`
	Error  *Error
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return e.Message
}

type Notification struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}
