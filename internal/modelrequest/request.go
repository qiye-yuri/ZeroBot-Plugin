// Package modelrequest completes non-streaming model HTTP responses before returning.
package modelrequest

import (
	"io"

	"github.com/fumiama/deepinfra"
)

// Request keeps a successfully decoded JSON reply from closing an unknown-length
// response early. The API's HTTP timeout also covers reading the remaining body.
func Request(api deepinfra.API, model deepinfra.Model) (string, error) {
	return api.Request(completeModel{model})
}

type completeModel struct{ deepinfra.Model }

func (m completeModel) Parse(body io.Reader) error {
	if err := m.Model.Parse(body); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, body)
	return err
}
