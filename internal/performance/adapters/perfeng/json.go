package perfeng

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// decodeValue rejects ambiguous duplicate members before schema validation.
func decodeValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 32 {
		return nil, errors.New("catalog JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, errors.New("malformed catalog JSON")
	}
	if token != json.Delim('{') && token != json.Delim('[') {
		return token, nil
	}
	object, array := map[string]any{}, []any{}
	for decoder.More() {
		var name string
		if token == json.Delim('{') {
			member, err := decoder.Token()
			if err != nil {
				return nil, errors.New("malformed catalog member")
			}
			name = member.(string)
			if _, exists := object[name]; exists {
				return nil, errors.New("duplicate catalog JSON member")
			}
		}
		value, err := decodeValue(decoder, depth+1)
		if err != nil {
			return nil, err
		}
		object[name], array = value, append(array, value)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.New("malformed catalog container")
	}
	if token == json.Delim('{') {
		return object, nil
	}

	return array, nil
}

// Go's JSON decoder replaces invalid surrogate escapes; the contract rejects them.
func validUnicode(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for index := 0; index+1 < len(data); index++ {
		if data[index] != '\\' {
			continue
		}
		index++
		if data[index] != 'u' || index+4 >= len(data) {
			continue
		}
		code, err := strconv.ParseUint(string(data[index+1:index+5]), 16, 16)
		if err != nil {
			return false
		}
		index += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
		if code < 0xd800 || code > 0xdbff {
			continue
		}
		if !lowSurrogate(data, index) {
			return false
		}
		index += 6
	}

	return true
}

func lowSurrogate(data []byte, index int) bool {
	if index+6 >= len(data) || string(data[index+1:index+3]) != `\u` {
		return false
	}
	low, err := strconv.ParseUint(string(data[index+3:index+7]), 16, 16)

	return err == nil && low >= 0xdc00 && low <= 0xdfff
}

// canonical implements the producer's restricted string/object/array/null format,
// including literal Unicode and HTML characters (encoding/json escapes U+2028).
func canonical(value any) []byte {
	var output strings.Builder
	writeCanonical(&output, value)
	return []byte(output.String())
}

func writeCanonical(output *strings.Builder, value any) {
	switch value := value.(type) {
	case nil:
		output.WriteString("null")
	case string:
		writeString(output, value)
	case []any:
		output.WriteByte('[')
		for index, item := range value {
			if index > 0 {
				output.WriteByte(',')
			}
			writeCanonical(output, item)
		}
		output.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		output.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				output.WriteByte(',')
			}
			writeCanonical(output, key)
			output.WriteByte(':')
			writeCanonical(output, value[key])
		}
		output.WriteByte('}')
	}
}

func writeString(output *strings.Builder, value string) {
	output.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"', '\\':
			output.WriteByte('\\')
			output.WriteRune(character)
		case '\b', '\f', '\n', '\r', '\t':
			output.WriteByte('\\')
			output.WriteByte(map[rune]byte{'\b': 'b', '\f': 'f', '\n': 'n', '\r': 'r', '\t': 't'}[character])
		default:
			if character < 32 {
				fmt.Fprintf(output, `\u%04x`, character)
			} else {
				output.WriteRune(character)
			}
		}
	}
	output.WriteByte('"')
}
