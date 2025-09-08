package main

import "testing"

func TestPythonFormat(t *testing.T) {
	expected := `{
    "foo" = {
        "bar" = <
            "q" = "e";
        > %true;
        "baz" = "qqq";
    };
}`

	actual, err := handle([]byte("{foo={bar=<q=e>%true;baz=qqq}}"), "pretty", "python", false)

	if err != nil {
		t.Errorf("Should not produce an error")
	}

	if expected != actual {
		t.Errorf("Result was incorrect, got: %s, want: %s.", actual, expected)
	}
}
