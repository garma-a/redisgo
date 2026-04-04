package resp

import (
	"fmt"
	"strconv"
	"strings"
)

func Parse(data []byte) ([]string, error) {
	// Imagine the client sends: *2\r\n$4\r\nECHO\r\n$2\r\nHI\r\n
	//
	// parts := strings.Split(str, "\r\n"):
	// This turns the string into an array: ["*2", "$4", "ECHO", "$2", "HI", ""].
	// if parts[0][0] == '*':
	// It checks the very first character. * tells the parser: "An Array is coming!"
	//
	// numElements, _ := strconv.Atoi(parts[0][1:]):
	// It looks at *2, skips the *, and converts the 2 into an integer. Now the code knows it needs to find 2 words
	// The for loop:
	//
	// First pass (i=0): It sees $4. It confirms it starts with $. It skips the $4 line and grabs the next line: "ECHO".
	//
	// Second pass (i=1): It sees $2. It skips it and grabs the next line: "HI".
	//
	// result = append(result, parts[idx]):
	// It adds these words to a Go slice: ["ECHO", "HI"].
	//
	// return result, nil:
	// It sends the clean list of words back to the handler.
	str := string(data)
	parts := strings.Split(str, "\r\n")
	if len(parts) < 1 {
		return nil, fmt.Errorf("empty input")
	}
	if parts[0][0] == '*' {
		numElements, err := strconv.Atoi(parts[0][1:])
		if err != nil {
			return nil, fmt.Errorf("invalid array format")
		}
		result := []string{}
		idx := 1
		for i := 0; i < numElements; i++ {
			if idx >= len(parts) {
				return nil, fmt.Errorf("incomplete data")
			}
			if parts[idx][0] != '$' {
				return nil, fmt.Errorf("expected bulk string")
			}
			_, err := strconv.Atoi(parts[idx][1:])
			if err != nil {
				return nil, fmt.Errorf("invalid length")
			}
			idx++
			if idx >= len(parts) {
				return nil, fmt.Errorf("incomplete data")
			}
			result = append(result, parts[idx])
			idx++
		}
		return result, nil
	}
	return nil, fmt.Errorf("unsupported format")
}
