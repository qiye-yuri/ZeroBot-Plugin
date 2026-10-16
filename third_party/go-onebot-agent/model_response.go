package goba

import (
	"errors"
	"io"
	"strings"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

var errModelResponseContent = errors.New("Agent model response has no valid text output")

// Request-local adapter: preserve protocol routing, headers and request body,
// but validate output before API.Request invokes Contents.String.
type agentResponseModel struct {
	deepinfra.Model
	contents model.Contents
}

func (m *agentResponseModel) Parse(body io.Reader) (err error) {
	m.contents = nil
	defer func() {
		// Some protocol adapters panic while converting unsupported response
		// parts. Keep this boundary limited to response parsing/extraction.
		if recover() != nil {
			m.contents = nil
			err = errModelResponseContent
		}
	}()
	if err = m.Model.Parse(body); err != nil {
		return err
	}
	text, err := agentResponseText(m.Model.Output())
	if err != nil {
		return err
	}
	m.contents = model.Contents{model.NewContentText(text)}
	return nil
}

func (m *agentResponseModel) Output() model.Contents    { return m.contents }
func (m *agentResponseModel) OutputRaw() model.Contents { return m.contents }

func agentResponseText(parts model.Contents) (string, error) {
	if len(parts) == 0 {
		return "", errModelResponseContent
	}
	size := 0
	for _, part := range parts {
		if part.Type != model.ContentTypeText {
			return "", errModelResponseContent
		}
		if len(part.Text) > agentActionOutputBytes-size {
			return "", ErrActionOutputLimit
		}
		size += len(part.Text)
	}
	var text strings.Builder
	text.Grow(size)
	for _, part := range parts {
		text.WriteString(part.Text)
	}
	return text.String(), nil
}
