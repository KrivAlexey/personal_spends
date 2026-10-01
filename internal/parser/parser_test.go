package parser

import "testing"

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		separator string
		want      float64
		wantErr   bool
	}{
		{name: "comma decimal", value: "-6,09", separator: ",", want: -6.09},
		{name: "comma decimal with thousands", value: "1.234,56", separator: ",", want: 1234.56},
		{name: "comma decimal whole number", value: "42", separator: ",", want: 42},
		{name: "dot decimal", value: "12.34", separator: ".", want: 12.34},
		{name: "padded", value: " -8,99 ", separator: ",", want: -8.99},
		{name: "not a number", value: "abc", separator: ",", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAmount(tt.value, tt.separator)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseAmount(%q, %q) error = %v, wantErr %v", tt.value, tt.separator, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseAmount(%q, %q) = %v, want %v", tt.value, tt.separator, got, tt.want)
			}
		})
	}
}
