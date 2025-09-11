package main

import "testing"

func TestPrettyFormat(t *testing.T) {
	expected := `{
    "foo" = {
        "bar" = <
            "q" = "e";
        > %true;
        "baz" = "qqq";
    };
}`

	actual, err := handle([]byte("{foo={bar=<q=e>%true;baz=qqq}}"), "pretty", "pretty", false)

	if err != nil {
		t.Errorf("Should not produce an error")
	}

	if expected != actual {
		t.Errorf("Result was incorrect, got: %s, want: %s.", actual, expected)
	}
}

func TestFloat(t *testing.T) {
	expected := `1234`

	actual, err := handle([]byte("1234"), "j2y", "compact", false)

	if err != nil {
		t.Errorf("Should not produce an error")
	}

	if expected != actual {
		t.Errorf("Result was incorrect, got: %s, want: %s.", actual, expected)
	}
}
