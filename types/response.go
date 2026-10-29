package types

// ResponseMeta carries safe HTTP and rate-limit metadata about a
// call. Headers contains only response headers — request headers
// (which carry credentials) are never stored or exposed.
type ResponseMeta struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Attempts   int               `json:"attempts"`
}

// Response is the typed result of a single API call: domain data
// plus safe transport metadata. Construct with NewResponse.
type Response[T any] struct {
	Data T            `json:"data"`
	Meta ResponseMeta `json:"meta"`
}

// NewResponse builds a Response from decoded data and metadata.
func NewResponse[T any](data T, meta ResponseMeta) *Response[T] {
	return &Response[T]{Data: data, Meta: meta}
}
