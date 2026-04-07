package resp

import (
	"reflect"
	"testing"
)

type testCase struct {
	name    string
	input   []byte
	want    []string
	wantErr bool
}

var tests = []testCase{
	{
		name:  "valid input",
		input: []byte("*2\r\n$4\r\nECHO\r\n$2\r\nHI\r\n"),
		want:  []string{"ECHO", "HI"},
	},
	{
		name:    "empty input",
		input:   []byte(""),
		wantErr: true,
	},
	{
		name:    "invalid array format",
		input:   []byte("*X\r\n$4\r\nECHO\r\n$2\r\nHI\r\n"),
		wantErr: true,
	},
	{
		name:    "incomplete data",
		input:   []byte("*2\r\n$4\r\nECHO\r\n"),
		wantErr: true,
	},
	{
		name:    "missing bulk string",
		input:   []byte("*2\r\n$4\r\nECHO\r\n$2\r\n"),
		wantErr: true,
	},
	{
		name:    "unsupported format",
		input:   []byte("ECHO HI\r\n"),
		wantErr: true,
	},
}

func TestParse(t *testing.T) {
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse() got = %v, want %v", got, tc.want)
			}
		})
	}

}
