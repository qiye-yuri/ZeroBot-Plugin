package goba

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Check decoded keys before decoding into maps/structs, which would merge or
// overwrite duplicates. Each object has its own key set, including array items.
type actionJSONBudget struct{ values int }

func (budget *actionJSONBudget) checkKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return budget.checkValueKeys(dec, true, 0)
}

func (budget *actionJSONBudget) checkValueKeys(dec *json.Decoder, root bool, depth int) error {
	budget.values++
	if budget.values > agentActionJSONValues {
		return ErrActionOutputLimit
	}
	token, err := dec.Token()
	if err != nil {
		return ErrActionOutputInvalid
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if depth >= agentActionJSONDepth {
		return ErrActionOutputLimit
	}
	var closeDelim json.Delim
	switch delim {
	case '{':
		closeDelim = '}'
		seen := map[string]struct{}{}
		for dec.More() {
			token, err := dec.Token()
			key, ok := token.(string)
			if err != nil || !ok {
				return ErrActionOutputInvalid
			}
			if root {
				// Match encoding/json's case-insensitive APIRequest fields;
				// ordinary parameter/message map keys remain case-sensitive.
				for _, field := range []string{"action", "params", "echo"} {
					if strings.EqualFold(key, field) {
						key = field
						break
					}
				}
			}
			if _, exists := seen[key]; exists {
				return ErrActionOutputInvalid
			}
			seen[key] = struct{}{}
			if err := budget.checkValueKeys(dec, false, depth+1); err != nil {
				return err
			}
		}
	case '[':
		closeDelim = ']'
		for dec.More() {
			if err := budget.checkValueKeys(dec, false, depth+1); err != nil {
				return err
			}
		}
	default:
		return ErrActionOutputInvalid
	}
	token, err = dec.Token()
	if err != nil || token != closeDelim {
		return ErrActionOutputInvalid
	}
	return nil
}
