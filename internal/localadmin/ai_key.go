package localadmin

import "errors"

// apply preserves the old value for keep. Callers using sparse updates must
// still omit that field, so keep cannot overwrite a concurrently changed key.
// Provider-specific key validation stays with the configuration being saved.
func (change AIKeyChange) apply(previous string) (string, error) {
	switch change.Action {
	case "keep":
		if change.Value != "" || change.Confirm {
			return "", errors.New("保留密钥无需新值或确认")
		}
		return previous, nil
	case "replace":
		if change.Value == "" || change.Confirm {
			return "", errors.New("替换密钥须填写新值")
		}
		return change.Value, nil
	case "clear":
		if !change.Confirm || change.Value != "" {
			return "", errors.New("请确认清除密钥")
		}
		return "", nil
	default:
		return "", errors.New("密钥操作无效")
	}
}
