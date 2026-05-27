package paths

import "testing"

func TestDecode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"-Users-alice", "/Users/alice"},
		{"-Users-alice-Documents-myproject", "/Users/alice/Documents/myproject"},
		{"-Users-alice-Documents-multi-word-project", "/Users/alice/Documents/multi/word/project"},
		{"", ""},
		{"no-leading-dash", "no/leading/dash"},
	}
	for _, c := range cases {
		got := Decode(c.in)
		if got != c.want {
			t.Errorf("Decode(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

func TestEncode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/Users/alice", "-Users-alice"},
		{"/Users/alice/Documents/myproject", "-Users-alice-Documents-myproject"},
	}
	for _, c := range cases {
		got := Encode(c.in)
		if got != c.want {
			t.Errorf("Encode(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}
