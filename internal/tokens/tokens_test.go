package tokens

import "testing"

func TestEstimate(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},        // ceil(1/4)
		{"abcd", 1},     // ceil(4/4)
		{"abcde", 2},    // ceil(5/4)
		{"12345678", 2}, // ceil(8/4)
		{"123456789", 3},
	}
	for _, c := range cases {
		if got := Estimate(c.in); got != c.want {
			t.Errorf("Estimate(%q) = %d; want %d", c.in, got, c.want)
		}
	}
}
