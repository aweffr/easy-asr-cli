package redact

import "regexp"

var urlWithQueryPattern = regexp.MustCompile(`https?(?:://|:\\/\\/)[^\s"'<>?]+\?[^\s"'<>]+`)

func URLQueries(text string) string {
	return urlWithQueryPattern.ReplaceAllStringFunc(text, func(value string) string {
		for i, r := range value {
			if r == '?' {
				return value[:i+1] + "<redacted>"
			}
		}
		return value
	})
}
