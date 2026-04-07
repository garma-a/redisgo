package resp

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse converts a raw RESP (Redis Serialization Protocol) byte slice into a slice of strings.
//
// It expects a RESP array format starting with '*', followed by the number of elements.
// Each element is expected to be a bulk string starting with '$', followed by its byte length,
// and then the actual string data on the next line.
//
// Example:
// If the client sends the raw bytes for: "*2\r\n$4\r\nECHO\r\n$2\r\nHI\r\n"
//
//  1. The parser splits the string by "\r\n":
//     ["*2", "$4", "ECHO", "$2", "HI", ""]
//  2. It reads "*2" to understand an array of 2 elements is coming.
//  3. It skips the byte length indicators ("$4" and "$2") and extracts the actual words.
//
// The returned slice for this example will be: ["ECHO", "HI"]
func Parse(data []byte) ([]string, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	str := string(data)
	parts := strings.Split(str, "\r\n")
	if len(parts) == 0 || len(parts[0]) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	if parts[0][0] == '*' {
		if len(parts[0]) < 2 {
			return nil, fmt.Errorf("invalid array format")
		}

		numElements, err := strconv.Atoi(parts[0][1:])
		if err != nil {
			return nil, fmt.Errorf("invalid array format")
		}
		result := []string{}
		idx := 1
		for range numElements {
			if idx >= len(parts) || len(parts[idx]) == 0 {
				return nil, fmt.Errorf("incomplete data")
			}
			if parts[idx][0] != '$' {
				return nil, fmt.Errorf("expected bulk string")
			}
			if len(parts[idx]) < 2 {
				return nil, fmt.Errorf("invalid length")
			}

			bulkLen, err := strconv.Atoi(parts[idx][1:])
			if err != nil || bulkLen < 0 {
				return nil, fmt.Errorf("invalid length")
			}
			idx++
			if idx >= len(parts) {
				return nil, fmt.Errorf("incomplete data")
			}
			if len(parts[idx]) != bulkLen {
				return nil, fmt.Errorf("invalid bulk string length")
			}
			result = append(result, parts[idx])
			idx++
		}
		return result, nil
	}
	return nil, fmt.Errorf("unsupported format")
}
