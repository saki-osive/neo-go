package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	// JSONRPCVersion is the only JSON-RPC protocol version supported.
	JSONRPCVersion = "2.0"
)

// RawParams is just a slice of abstract values, used to represent parameters
// passed from client to server.
type RawParams struct {
	Values []interface{}
}

// NewRawParams creates RawParams from its parameters.
func NewRawParams(vals ...interface{}) RawParams {
	p := RawParams{}
	p.Values = make([]interface{}, len(vals))
	for i := 0; i < len(p.Values); i++ {
		p.Values[i] = vals[i]
	}
	return p
}

// Raw represents JSON-RPC request.
type Raw struct {
	JSONRPC   string        `json:"jsonrpc"`
	Method    string        `json:"method"`
	RawParams []interface{} `json:"params"`
	ID        int           `json:"id"`
}

// In contains standard JSON-RPC 2.0 request and batch of
// requests: http://www.jsonrpc.org/specification.
// It's used in server to represent incoming queries.
type In struct {
	Request *Request
	Batch   Batch
}

// Request represents a standard JSON-RPC 2.0
// request: http://www.jsonrpc.org/specification#request_object.
type Request struct {
	JSONRPC   string          `json:"jsonrpc"`
	Method    string          `json:"method"`
	RawParams json.RawMessage `json:"params,omitempty"`
	RawID     json.RawMessage `json:"id,omitempty"`
	Error     error           `json:"-"`
}

// Batch represents a standard JSON-RPC 2.0
// batch: https://www.jsonrpc.org/specification#batch.
type Batch []*Request

// MarshalJSON implement JSON Marshaller interface
func (i In) MarshalJSON() ([]byte, error) {
	if i.Request != nil {
		return json.Marshal(i.Request)
	}
	return json.Marshal(i.Batch)
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (i *In) UnmarshalJSON(data []byte) error {
	var (
		req   *Request
		batch Batch
	)
	switch data[0] {
	case '[':
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.Token() // `[` and `]`
		for decoder.More() {
			r := NewRequest()
			decodeErr := decoder.Decode(r)
			if decodeErr != nil {
				r.Error = decodeErr
			} else if r.JSONRPC != JSONRPCVersion {
				r.Error = fmt.Errorf("invalid version, expected 2.0 got: '%s'", r.JSONRPC)
			}
			batch = append(batch, r)
		}
		if len(batch) == 0 {
			r := NewRequest()
			r.Error = errors.New("empty request")
			batch = append(batch, r)
		}
	case '{':
		req = NewRequest()
		err := json.Unmarshal(data, req)
		if err != nil {
			return fmt.Errorf("error parsing JSON payload: %w", err)
		}
		if req.JSONRPC != JSONRPCVersion {
			return fmt.Errorf("invalid version, expected 2.0 got: '%s'", req.JSONRPC)
		}
	default:
		return errors.New("unexpected JSON format")
	}

	i.Request = req
	i.Batch = batch
	return nil
}

// DecodeData decodes the given reader into the the request
// struct.
func (i *In) DecodeData(data io.ReadCloser) error {
	defer data.Close()

	rawData := json.RawMessage{}
	err := json.NewDecoder(data).Decode(&rawData)
	if err != nil {
		return fmt.Errorf("error parsing JSON payload: %w", err)
	}

	return i.UnmarshalJSON(rawData)
}

// NewIn creates a new In struct.
func NewIn() *In {
	return &In{}
}

// NewRequest creates a new Request struct.
func NewRequest() *Request {
	return &Request{
		JSONRPC: JSONRPCVersion,
	}
}

// Params takes a slice of any type and attempts to bind
// the params to it.
func (r *Request) Params() (*Params, error) {
	params := Params{}

	err := json.Unmarshal(r.RawParams, &params)
	if err != nil {
		return nil, fmt.Errorf("error parsing params: %w", err)
	}

	return &params, nil
}
